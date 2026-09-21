// SPDX-License-Identifier: TODO

package systemctl

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/m4schini/robbe/internal/testutil"
)

func TestDaemonReload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user bool
		want string
	}{
		{name: "user", user: true, want: "systemctl --user daemon-reload"},
		{name: "system", user: false, want: "systemctl daemon-reload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := testutil.NewFakeRunner()
			m := New(runner, tt.user)

			if err := m.DaemonReload(t.Context()); err != nil {
				t.Fatalf("DaemonReload() error = %v", err)
			}

			lines := runner.Lines()
			if len(lines) != 1 || lines[0] != tt.want {
				t.Fatalf("Lines() = %v, want [%q]", lines, tt.want)
			}
		})
	}
}

func TestUnitVerbs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user bool
		verb string
		run  func(m *Manager) error
		want string
	}{
		{
			name: "start user",
			user: true,
			verb: "start",
			run: func(m *Manager) error {
				return m.Start(t.Context(), "a.service", "b.service")
			},
			want: "systemctl --user start a.service b.service",
		},
		{
			name: "start system",
			user: false,
			verb: "start",
			run: func(m *Manager) error {
				return m.Start(t.Context(), "a.service", "b.service")
			},
			want: "systemctl start a.service b.service",
		},
		{
			name: "restart user",
			user: true,
			verb: "restart",
			run: func(m *Manager) error {
				return m.Restart(t.Context(), "a.service", "b.service")
			},
			want: "systemctl --user restart a.service b.service",
		},
		{
			name: "restart system",
			user: false,
			verb: "restart",
			run: func(m *Manager) error {
				return m.Restart(t.Context(), "a.service", "b.service")
			},
			want: "systemctl restart a.service b.service",
		},
		{
			name: "stop user",
			user: true,
			verb: "stop",
			run: func(m *Manager) error {
				return m.Stop(t.Context(), "a.service", "b.service")
			},
			want: "systemctl --user stop a.service b.service",
		},
		{
			name: "stop system",
			user: false,
			verb: "stop",
			run: func(m *Manager) error {
				return m.Stop(t.Context(), "a.service", "b.service")
			},
			want: "systemctl stop a.service b.service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := testutil.NewFakeRunner()
			m := New(runner, tt.user)

			if err := tt.run(m); err != nil {
				t.Fatalf("%s() error = %v", tt.verb, err)
			}

			lines := runner.Lines()
			if len(lines) != 1 || lines[0] != tt.want {
				t.Fatalf("Lines() = %v, want [%q]", lines, tt.want)
			}
		})
	}
}

func TestUnitVerbs_Empty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(m *Manager) error
	}{
		{name: "start", run: func(m *Manager) error { return m.Start(t.Context()) }},
		{name: "restart", run: func(m *Manager) error { return m.Restart(t.Context()) }},
		{name: "stop", run: func(m *Manager) error { return m.Stop(t.Context()) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := testutil.NewFakeRunner()
			m := New(runner, true)

			if err := tt.run(m); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}

			if calls := runner.Calls(); len(calls) != 0 {
				t.Errorf("Calls() = %v, want none", calls)
			}
		})
	}
}

func TestStart_Failure(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Fail("systemctl start a.service", "Failed to start a.service: Unit not found.")

	m := New(runner, false)

	err := m.Start(t.Context(), "a.service")
	if err == nil {
		t.Fatalf("Start() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "Failed to start a.service: Unit not found.") {
		t.Errorf("err = %q, want to contain stderr", err.Error())
	}

	if !strings.Contains(err.Error(), "systemctl start") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "systemctl start")
	}
}

func TestActiveStates_AllActive(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Script("systemctl is-active a.service b.service", testutil.Result{
		Stdout: "active\nactive\n", //nolint:dupword // two units, both active
		Stderr: "",
		Err:    nil,
	})

	m := New(runner, false)

	got, err := m.ActiveStates(t.Context(), "a.service", "b.service")
	if err != nil {
		t.Fatalf("ActiveStates() error = %v", err)
	}

	want := map[string]string{"a.service": "active", "b.service": "active"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveStates() = %v, want %v", got, want)
	}
}

func TestActiveStates_MixedWithNonZeroExit(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Script("systemctl is-active a.service b.service c.service", testutil.Result{
		Stdout: "active\ninactive\nfailed\n",
		Stderr: "",
		Err:    testutil.ErrExit,
	})

	m := New(runner, false)

	got, err := m.ActiveStates(t.Context(), "a.service", "b.service", "c.service")
	if err != nil {
		t.Fatalf("ActiveStates() error = %v, want nil", err)
	}

	want := map[string]string{"a.service": "active", "b.service": "inactive", "c.service": "failed"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveStates() = %v, want %v", got, want)
	}
}

func TestActiveStates_Empty(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	m := New(runner, false)

	got, err := m.ActiveStates(t.Context())
	if err != nil {
		t.Fatalf("ActiveStates() error = %v", err)
	}

	if len(got) != 0 {
		t.Errorf("ActiveStates() = %v, want empty", got)
	}

	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("Calls() = %v, want none", calls)
	}
}

func TestActiveStates_WrongLineCountWithError(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Script("systemctl is-active a.service b.service", testutil.Result{
		Stdout: "active\n",
		Stderr: "",
		Err:    testutil.ErrExit,
	})

	m := New(runner, false)

	_, err := m.ActiveStates(t.Context(), "a.service", "b.service")
	if err == nil {
		t.Fatalf("ActiveStates() error = nil, want error")
	}
}

func TestActiveStates_EmptyStdoutWithError(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Fail("systemctl is-active a.service", "Failed to connect to bus: No such file or directory")

	m := New(runner, false)

	got, err := m.ActiveStates(t.Context(), "a.service")
	if err == nil {
		t.Fatalf("ActiveStates() = %v, error = nil, want error", got)
	}

	if !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Errorf("err = %q, want to contain stderr", err.Error())
	}
}

func TestActiveStates_EmptyStdoutWithoutError(t *testing.T) {
	t.Parallel()

	runner := testutil.NewFakeRunner()
	runner.Script("systemctl is-active a.service", testutil.Result{
		Stdout: "",
		Stderr: "",
		Err:    nil,
	})

	m := New(runner, false)

	got, err := m.ActiveStates(t.Context(), "a.service")
	if !errors.Is(err, ErrUnexpectedOutput) {
		t.Fatalf("ActiveStates() = %v, error = %v, want %v", got, err, ErrUnexpectedOutput)
	}
}
