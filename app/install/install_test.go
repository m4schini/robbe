// SPDX-License-Identifier: TODO

package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m4schini/robbe/internal/testutil"
	"go.uber.org/zap"
)

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		opts := Options{
			User:     true,
			UnitDir:  "/x",
			Binary:   "/usr/local/bin/robbe",
			Interval: 5 * time.Minute,
		}

		service, timer, err := Render(opts)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if !strings.Contains(string(service), "Type=oneshot") {
			t.Errorf("service = %q, want to contain %q", service, "Type=oneshot")
		}

		if !strings.Contains(string(service), "ExecStart=/usr/local/bin/robbe sync") {
			t.Errorf("service = %q, want to contain %q", service, "ExecStart=/usr/local/bin/robbe sync")
		}

		// systemd creates the four directories (XDG mapped for user units).
		for _, want := range []string{"ConfigurationDirectory=robbe", "CacheDirectory=robbe", "StateDirectory=robbe", "RuntimeDirectory=robbe"} {
			if !strings.Contains(string(service), want) {
				t.Errorf("service = %q, want to contain %q", service, want)
			}
		}

		for _, want := range []string{"OnUnitActiveSec=300s", "OnBootSec=1min", "Persistent=true", "WantedBy=timers.target"} {
			if !strings.Contains(string(timer), want) {
				t.Errorf("timer = %q, want to contain %q", timer, want)
			}
		}
	})

	t.Run("interval rounds to seconds", func(t *testing.T) {
		t.Parallel()

		opts := Options{
			User:     false,
			UnitDir:  "/x",
			Binary:   "/usr/local/bin/robbe",
			Interval: 90 * time.Second,
		}

		_, timer, err := Render(opts)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if !strings.Contains(string(timer), "OnUnitActiveSec=90s") {
			t.Errorf("timer = %q, want to contain %q", timer, "OnUnitActiveSec=90s")
		}
	})

	t.Run("zero interval errors", func(t *testing.T) {
		t.Parallel()

		opts := Options{
			User:     false,
			UnitDir:  "/x",
			Binary:   "/usr/local/bin/robbe",
			Interval: 0,
		}

		_, _, err := Render(opts)
		if !errors.Is(err, ErrInterval) {
			t.Fatalf("Render() error = %v, want ErrInterval", err)
		}
	})
}

func TestUnitDir(t *testing.T) {
	if got, want := UnitDir(false, "/home/x"), "/etc/systemd/system"; got != want {
		t.Errorf("UnitDir(false, ...) = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")

	if got, want := UnitDir(true, "/home/x"), filepath.Join("/home/x", ".config", "systemd", "user"); got != want {
		t.Errorf("UnitDir(true, ...) = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	if got, want := UnitDir(true, "/home/x"), filepath.Join("/xdg", "systemd", "user"); got != want {
		t.Errorf("UnitDir(true, ...) = %q, want %q", got, want)
	}

	// A relative XDG_CONFIG_HOME must be ignored per the XDG spec.
	t.Setenv("XDG_CONFIG_HOME", "relative/xdg")

	if got, want := UnitDir(true, "/home/x"), filepath.Join("/home/x", ".config", "systemd", "user"); got != want {
		t.Errorf("UnitDir(true, ...) with relative XDG_CONFIG_HOME = %q, want %q", got, want)
	}
}

func TestInstall_User(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "units")
	opts := Options{
		User:     true,
		UnitDir:  dir,
		Binary:   "/usr/local/bin/robbe",
		Interval: 5 * time.Minute,
	}

	wantService, wantTimer, err := Render(opts)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	runner := testutil.NewFakeRunner()
	installer := &Installer{
		Runner: runner,
		Opts:   opts,
		Log:    zap.NewNop(),
	}

	if err := installer.Install(t.Context()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	gotService, err := os.ReadFile(filepath.Join(dir, ServiceUnit))
	if err != nil {
		t.Fatalf("read %s: %v", ServiceUnit, err)
	}

	if string(gotService) != string(wantService) {
		t.Errorf("service content = %q, want %q", gotService, wantService)
	}

	gotTimer, err := os.ReadFile(filepath.Join(dir, TimerUnit))
	if err != nil {
		t.Fatalf("read %s: %v", TimerUnit, err)
	}

	if string(gotTimer) != string(wantTimer) {
		t.Errorf("timer content = %q, want %q", gotTimer, wantTimer)
	}

	want := []string{"systemctl --user daemon-reload", "systemctl --user enable --now robbe-sync.timer"}
	if got := runner.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %v, want %v", got, want)
	}
}

func TestInstall_System(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "units")
	opts := Options{
		User:     false,
		UnitDir:  dir,
		Binary:   "/usr/local/bin/robbe",
		Interval: 5 * time.Minute,
	}

	runner := testutil.NewFakeRunner()
	installer := &Installer{
		Runner: runner,
		Opts:   opts,
		Log:    zap.NewNop(),
	}

	if err := installer.Install(t.Context()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	want := []string{"systemctl daemon-reload", "systemctl enable --now robbe-sync.timer"}
	if got := runner.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %v, want %v", got, want)
	}
}

func TestInstall_EnableFails(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "units")
	opts := Options{
		User:     true,
		UnitDir:  dir,
		Binary:   "/usr/local/bin/robbe",
		Interval: 5 * time.Minute,
	}

	runner := testutil.NewFakeRunner()
	runner.Fail("systemctl --user enable --now robbe-sync.timer", "Failed to enable")

	installer := &Installer{
		Runner: runner,
		Opts:   opts,
		Log:    zap.NewNop(),
	}

	err := installer.Install(t.Context())
	if err == nil {
		t.Fatalf("Install() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "Failed to enable") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "Failed to enable")
	}

	if _, statErr := os.Stat(filepath.Join(dir, ServiceUnit)); statErr != nil {
		t.Errorf("stat %s error = %v, want file to exist", ServiceUnit, statErr)
	}

	if _, statErr := os.Stat(filepath.Join(dir, TimerUnit)); statErr != nil {
		t.Errorf("stat %s error = %v, want file to exist", TimerUnit, statErr)
	}
}

func TestUninstall(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		opts := Options{
			User:     true,
			UnitDir:  dir,
			Binary:   "/usr/local/bin/robbe",
			Interval: 5 * time.Minute,
		}

		if err := os.WriteFile(filepath.Join(dir, ServiceUnit), []byte("service"), filePerm); err != nil {
			t.Fatalf("write %s: %v", ServiceUnit, err)
		}

		if err := os.WriteFile(filepath.Join(dir, TimerUnit), []byte("timer"), filePerm); err != nil {
			t.Fatalf("write %s: %v", TimerUnit, err)
		}

		runner := testutil.NewFakeRunner()
		installer := &Installer{
			Runner: runner,
			Opts:   opts,
			Log:    zap.NewNop(),
		}

		if err := installer.Uninstall(t.Context()); err != nil {
			t.Fatalf("Uninstall() error = %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, ServiceUnit)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s error = %v, want ErrNotExist", ServiceUnit, err)
		}

		if _, err := os.Stat(filepath.Join(dir, TimerUnit)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s error = %v, want ErrNotExist", TimerUnit, err)
		}

		want := []string{"systemctl --user disable --now robbe-sync.timer", "systemctl --user daemon-reload"}
		if got := runner.Lines(); !reflect.DeepEqual(got, want) {
			t.Errorf("Lines() = %v, want %v", got, want)
		}
	})

	t.Run("disable fails", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		opts := Options{
			User:     true,
			UnitDir:  dir,
			Binary:   "/usr/local/bin/robbe",
			Interval: 5 * time.Minute,
		}

		if err := os.WriteFile(filepath.Join(dir, ServiceUnit), []byte("service"), filePerm); err != nil {
			t.Fatalf("write %s: %v", ServiceUnit, err)
		}

		if err := os.WriteFile(filepath.Join(dir, TimerUnit), []byte("timer"), filePerm); err != nil {
			t.Fatalf("write %s: %v", TimerUnit, err)
		}

		runner := testutil.NewFakeRunner()
		runner.Fail("systemctl --user disable --now robbe-sync.timer", "Failed to disable")

		installer := &Installer{
			Runner: runner,
			Opts:   opts,
			Log:    zap.NewNop(),
		}

		if err := installer.Uninstall(t.Context()); err != nil {
			t.Fatalf("Uninstall() error = %v, want nil", err)
		}

		if _, err := os.Stat(filepath.Join(dir, ServiceUnit)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s error = %v, want ErrNotExist", ServiceUnit, err)
		}

		if _, err := os.Stat(filepath.Join(dir, TimerUnit)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s error = %v, want ErrNotExist", TimerUnit, err)
		}

		lines := runner.Lines()

		found := false

		for _, line := range lines {
			if line == "systemctl --user daemon-reload" {
				found = true
			}
		}

		if !found {
			t.Errorf("Lines() = %v, want to contain %q", lines, "systemctl --user daemon-reload")
		}
	})
}

func TestUninstall_MissingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	opts := Options{
		User:     true,
		UnitDir:  dir,
		Binary:   "/usr/local/bin/robbe",
		Interval: 5 * time.Minute,
	}

	runner := testutil.NewFakeRunner()
	installer := &Installer{
		Runner: runner,
		Opts:   opts,
		Log:    zap.NewNop(),
	}

	if err := installer.Uninstall(t.Context()); err != nil {
		t.Fatalf("Uninstall() error = %v, want nil", err)
	}
}

func TestLingerEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stdout string
		want   bool
	}{
		{name: "yes", stdout: "Linger=yes\n", want: true},
		{name: "no", stdout: "Linger=no\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := testutil.NewFakeRunner()
			runner.Script("loginctl show-user 1000 -p Linger", testutil.Result{
				Stdout: tt.stdout,
				Stderr: "",
				Err:    nil,
			})

			installer := &Installer{
				Runner: runner,
				Opts: Options{
					User:     true,
					UnitDir:  "/x",
					Binary:   "/usr/local/bin/robbe",
					Interval: 5 * time.Minute,
				},
				Log: zap.NewNop(),
			}

			got, err := installer.LingerEnabled(t.Context(), 1000)
			if err != nil {
				t.Fatalf("LingerEnabled() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("LingerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("failure", func(t *testing.T) {
		t.Parallel()

		runner := testutil.NewFakeRunner()
		runner.Fail("loginctl show-user 1000 -p Linger", "no such user")

		installer := &Installer{
			Runner: runner,
			Opts: Options{
				User:     true,
				UnitDir:  "/x",
				Binary:   "/usr/local/bin/robbe",
				Interval: 5 * time.Minute,
			},
			Log: zap.NewNop(),
		}

		_, err := installer.LingerEnabled(t.Context(), 1000)
		if err == nil {
			t.Fatalf("LingerEnabled() error = nil, want error")
		}
	})
}
