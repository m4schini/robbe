// SPDX-License-Identifier: TODO

// Package install writes the robbe-sync service and timer units and
// enables the timer, so robbe runs itself periodically.
package install

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/m4schini/robbe/internal/xdg"
	"github.com/m4schini/robbe/ports"
	"go.uber.org/zap"
)

//go:embed templates/*
var templates embed.FS

const (
	// ServiceUnit is the name of the oneshot service.
	ServiceUnit = "robbe-sync.service"
	// TimerUnit is the name of the timer that triggers ServiceUnit.
	TimerUnit = "robbe-sync.timer"

	filePerm = 0o644
	dirPerm  = 0o755
)

// ErrInterval is returned for an interval that is not a positive whole
// number of seconds.
var ErrInterval = errors.New("invalid interval")

// Options configure where and how the units are installed.
type Options struct {
	// User selects the systemd user instance.
	User bool
	// UnitDir is the directory the units are written to; see UnitDir.
	UnitDir string
	// Binary is the absolute path of the robbe executable.
	Binary string
	// Interval is the time between two sync runs.
	Interval time.Duration
}

// Installer performs Install and Uninstall through a ports.Runner.
type Installer struct {
	Runner ports.Runner
	Opts   Options
	Log    *zap.Logger
}

// UnitDir returns the default unit directory for the scope: the user
// instance's config dir or /etc/systemd/system.
func UnitDir(user bool, home string) string {
	if !user {
		return "/etc/systemd/system"
	}

	return filepath.Join(xdg.ConfigHome(home), "systemd", "user")
}

// Render returns the content of both units for opts.
func Render(opts Options) (service, timer []byte, err error) {
	if opts.Interval < time.Second || opts.Interval%time.Second != 0 {
		return nil, nil, fmt.Errorf("%w: %s (must be whole seconds, at least 1s)", ErrInterval, opts.Interval)
	}

	data := struct {
		Binary   string
		Interval string
	}{
		Binary:   opts.Binary,
		Interval: strconv.FormatInt(int64(opts.Interval/time.Second), 10) + "s",
	}

	service, err = render(ServiceUnit, data)
	if err != nil {
		return nil, nil, err
	}

	timer, err = render(TimerUnit, data)
	if err != nil {
		return nil, nil, err
	}

	return service, timer, nil
}

func render(name string, data any) ([]byte, error) {
	tmpl, err := template.ParseFS(templates, "templates/"+name)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}

	return buf.Bytes(), nil
}

// Install writes both units, reloads systemd and enables the timer.
func (i *Installer) Install(ctx context.Context) error {
	service, timer, err := Render(i.Opts)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(i.Opts.UnitDir, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", i.Opts.UnitDir, err)
	}

	for name, content := range map[string][]byte{ServiceUnit: service, TimerUnit: timer} {
		path := filepath.Join(i.Opts.UnitDir, name)
		if err := os.WriteFile(path, content, filePerm); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}

		i.logger().Info("unit written", zap.String("path", path))
	}

	if err := i.systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}

	return i.systemctl(ctx, "enable", "--now", TimerUnit)
}

// Uninstall disables the timer, removes both units and reloads systemd.
func (i *Installer) Uninstall(ctx context.Context) error {
	if err := i.systemctl(ctx, "disable", "--now", TimerUnit); err != nil {
		i.logger().Warn("disable timer", zap.Error(err))
	}

	for _, name := range []string{TimerUnit, ServiceUnit} {
		path := filepath.Join(i.Opts.UnitDir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}

		i.logger().Info("unit removed", zap.String("path", path))
	}

	return i.systemctl(ctx, "daemon-reload")
}

// LingerEnabled reports whether the user instance keeps running without a
// login session (loginctl enable-linger). Only meaningful for user scope.
func (i *Installer) LingerEnabled(ctx context.Context, uid int) (bool, error) {
	stdout, stderr, err := i.Runner.Run(ctx, "loginctl", nil, "show-user", strconv.Itoa(uid), "-p", "Linger")
	if err != nil {
		return false, fmt.Errorf("loginctl show-user: %w: %s", err, strings.TrimSpace(string(stderr)))
	}

	value := strings.TrimSpace(string(stdout))
	value = strings.TrimPrefix(value, "Linger=")

	return value == "yes", nil
}

func (i *Installer) systemctl(ctx context.Context, args ...string) error {
	full := args
	if i.Opts.User {
		full = append([]string{"--user"}, args...)
	}

	_, stderr, err := i.Runner.Run(ctx, "systemctl", nil, full...)
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", args[0], err, strings.TrimSpace(string(stderr)))
	}

	return nil
}

func (i *Installer) logger() *zap.Logger {
	if i.Log == nil {
		return zap.NewNop()
	}

	return i.Log
}
