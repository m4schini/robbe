// SPDX-License-Identifier: TODO

// Package systemctl implements adapters.UnitManager with the systemctl binary.
package systemctl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/m4schini/robbe/adapters"
)

const bin = "systemctl"

// ErrUnexpectedOutput is returned when `systemctl is-active` does not print
// one state per unit.
var ErrUnexpectedOutput = errors.New("unexpected systemctl output")

// Manager drives systemctl through a adapters.Runner.
type Manager struct {
	runner adapters.Runner
	user   bool
}

// New returns a Manager. user selects `systemctl --user`.
func New(runner adapters.Runner, user bool) *Manager {
	return &Manager{runner: runner, user: user}
}

// DaemonReload implements adapters.UnitManager.
func (m *Manager) DaemonReload(ctx context.Context) error {
	_, err := m.run(ctx, "daemon-reload")

	return err
}

// Start implements adapters.UnitManager.
func (m *Manager) Start(ctx context.Context, units ...string) error {
	return m.units(ctx, "start", units)
}

// Restart implements adapters.UnitManager.
func (m *Manager) Restart(ctx context.Context, units ...string) error {
	return m.units(ctx, "restart", units)
}

// Stop implements adapters.UnitManager.
func (m *Manager) Stop(ctx context.Context, units ...string) error {
	return m.units(ctx, "stop", units)
}

// ActiveStates implements adapters.UnitManager. `systemctl is-active` prints
// one state per line and exits non-zero when any unit is not active, so the
// exit code is ignored as long as the output has one line per unit.
func (m *Manager) ActiveStates(ctx context.Context, units ...string) (map[string]string, error) {
	states := map[string]string{}
	if len(units) == 0 {
		return states, nil
	}

	stdout, err := m.run(ctx, "is-active", units...)

	out := strings.TrimSpace(string(stdout))
	if out == "" {
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("%w: is-active printed nothing for %d units", ErrUnexpectedOutput, len(units))
	}

	lines := strings.Split(out, "\n")
	if len(lines) != len(units) {
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("%w: is-active: expected %d lines, got %d", ErrUnexpectedOutput, len(units), len(lines))
	}

	for i, unit := range units {
		states[unit] = strings.TrimSpace(lines[i])
	}

	return states, nil
}

func (m *Manager) units(ctx context.Context, verb string, units []string) error {
	if len(units) == 0 {
		return nil
	}

	_, err := m.run(ctx, verb, units...)

	return err
}

func (m *Manager) run(ctx context.Context, verb string, args ...string) ([]byte, error) {
	full := make([]string, 0, len(args)+2) //nolint:mnd // --user and verb
	if m.user {
		full = append(full, "--user")
	}

	full = append(append(full, verb), args...)

	stdout, stderr, err := m.runner.Run(ctx, bin, nil, full...)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg != "" {
			return stdout, fmt.Errorf("systemctl %s: %w: %s", verb, err, msg)
		}

		return stdout, fmt.Errorf("systemctl %s: %w", verb, err)
	}

	return stdout, nil
}
