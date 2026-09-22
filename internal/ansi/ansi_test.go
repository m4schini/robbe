// SPDX-License-Identifier: TODO

package ansi

import (
	"errors"
	"os"
	"testing"
)

// tty opens the controlling terminal read-only, or skips the test when
// there is none (CI runs without one).
func tty(t *testing.T) *os.File {
	t.Helper()

	f, err := os.Open("/dev/tty")
	if err != nil {
		t.Skipf("no controlling terminal: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	return f
}

func tempFile(t *testing.T) *os.File {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "ansi")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	return f
}

func TestEnabled(t *testing.T) {
	// t.Setenv is used, so this test must not run in parallel.
	tests := []struct {
		name    string
		mode    string
		env     map[string]string
		file    func(*testing.T) *os.File
		want    bool
		wantErr error
	}{
		{name: "always ignores NO_COLOR and nil file", mode: ModeAlways, env: map[string]string{"NO_COLOR": "1"}, file: nil, want: true, wantErr: nil},
		{name: "never on a tty", mode: ModeNever, env: nil, file: tty, want: false, wantErr: nil},
		{name: "auto with empty NO_COLOR", mode: ModeAuto, env: map[string]string{"NO_COLOR": ""}, file: tty, want: false, wantErr: nil},
		{name: "auto with TERM=dumb", mode: ModeAuto, env: map[string]string{"TERM": "dumb"}, file: tty, want: false, wantErr: nil},
		{name: "auto with regular file", mode: ModeAuto, env: nil, file: tempFile, want: false, wantErr: nil},
		{name: "auto with nil file", mode: ModeAuto, env: nil, file: nil, want: false, wantErr: nil},
		{name: "auto with tty", mode: ModeAuto, env: nil, file: tty, want: true, wantErr: nil},
		{name: "invalid mode", mode: "blue", env: nil, file: nil, want: false, wantErr: ErrInvalidMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start from a clean environment so the developer's shell does
			// not leak into the auto branch. t.Setenv registers the restore
			// of the original NO_COLOR before it is unset.
			t.Setenv("NO_COLOR", "")
			os.Unsetenv("NO_COLOR")
			t.Setenv("TERM", "xterm")

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var f *os.File
			if tt.file != nil {
				f = tt.file(t)
			}

			got, err := Enabled(tt.mode, f)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Enabled() error = %v, want %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("Enabled(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestValidMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{ModeAuto, ModeAlways, ModeNever} {
		if err := ValidMode(mode); err != nil {
			t.Errorf("ValidMode(%q) = %v, want nil", mode, err)
		}
	}

	err := ValidMode("blue")
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("ValidMode(blue) = %v, want ErrInvalidMode", err)
	}
}
