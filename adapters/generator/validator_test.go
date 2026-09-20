// SPDX-License-Identifier: TODO

package generator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m4schini/robbe/internal/testutil"
)

const bin = "quadlet-generator"

func TestValidate_ParsesUnits(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	tests := []struct {
		name   string
		stdout string
		want   map[string]string
	}{
		{
			name: "single container",
			stdout: fmt.Sprintf(`---nginx.service---
[Unit]
SourcePath=%s
RequiresMountsFor=%%t/containers

[Service]
ExecStart=/usr/bin/podman run nginx

`, filepath.Join(dir, "nginx.container")),
			want: map[string]string{"nginx.container": "nginx.service"},
		},
		{
			name: "multiple units",
			stdout: fmt.Sprintf(`---nginx.service---
[Unit]
SourcePath=%s
RequiresMountsFor=%%t/containers

---cache.service---
[Unit]
SourcePath=%s
RequiresMountsFor=%%t/containers

[Service]
ExecStart=/usr/bin/podman run redis

---proxy-network.service---
[Unit]
SourcePath=%s

---web-pod.service---
[Unit]
SourcePath=%s

---data-volume.service---
[Unit]
SourcePath=%s

`,
				filepath.Join(dir, "nginx.container"),
				filepath.Join(dir, "sub", "redis.container"),
				filepath.Join(dir, "proxy.network"),
				filepath.Join(dir, "web.pod"),
				filepath.Join(dir, "data.volume"),
			),
			want: map[string]string{
				"nginx.container":     "nginx.service",
				"sub/redis.container": "cache.service",
				"proxy.network":       "proxy-network.service",
				"web.pod":             "web-pod.service",
				"data.volume":         "data-volume.service",
			},
		},
		{
			name:   "empty output",
			stdout: "",
			want:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := testutil.NewFakeRunner()
			runner.Script(bin, testutil.Result{Stdout: tt.stdout, Stderr: "", Err: nil})

			v := New(bin, runner)

			got, err := v.Validate(t.Context(), dir, true)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if !reflect.DeepEqual(got.Units, tt.want) {
				t.Errorf("Units = %v, want %v", got.Units, tt.want)
			}
		})
	}
}

func TestValidate_Args(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user bool
		want string
	}{
		{name: "user", user: true, want: bin + " -user -dryrun"},
		{name: "system", user: false, want: bin + " -dryrun"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			runner := testutil.NewFakeRunner()
			runner.Script(bin, testutil.Result{Stdout: "", Stderr: "", Err: nil})

			v := New(bin, runner)

			if _, err := v.Validate(t.Context(), dir, tt.user); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			lines := runner.Lines()
			if len(lines) != 1 || lines[0] != tt.want {
				t.Fatalf("Lines() = %v, want [%q]", lines, tt.want)
			}

			calls := runner.Calls()
			wantEnv := []string{"QUADLET_UNIT_DIRS=" + dir}
			if !reflect.DeepEqual(calls[0].Env, wantEnv) {
				t.Errorf("Env = %v, want %v", calls[0].Env, wantEnv)
			}
		})
	}
}

func TestValidate_Error(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runner := testutil.NewFakeRunner()

	stdout := fmt.Sprintf(`---good.service---
[Unit]
SourcePath=%s

[Service]
ExecStart=/usr/bin/podman run good

`, filepath.Join(dir, "good.container"))

	const stderr = `quadlet-generator[1454315]: Loading source unit file /abs/dir/bad.container
quadlet-generator[1454315]: converting "bad.container": unsupported key 'Img' in group 'Container' in /abs/dir/bad.container
quadlet-generator[1454315]: processing encountered some errors
`

	runner.Script(bin, testutil.Result{Stdout: stdout, Stderr: stderr, Err: testutil.ErrExit})

	v := New(bin, runner)

	report, err := v.Validate(t.Context(), dir, true)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want wrapping ErrValidation", err)
	}

	msg := err.Error()
	if !strings.Contains(msg, `converting "bad.container": unsupported key 'Img'`) {
		t.Errorf("err.Error() = %q, want to contain converting message", msg)
	}

	if strings.Contains(msg, "Loading source unit file") {
		t.Errorf("err.Error() = %q, want not to contain %q", msg, "Loading source unit file")
	}

	wantUnits := map[string]string{"good.container": "good.service"}
	if !reflect.DeepEqual(report.Units, wantUnits) {
		t.Errorf("Units = %v, want %v (partial map on failure)", report.Units, wantUnits)
	}
}

func TestValidate_ErrorWithoutConvertingLine(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runner := testutil.NewFakeRunner()
	runner.Fail(bin, "boom")

	v := New(bin, runner)

	_, err := v.Validate(t.Context(), dir, true)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want to contain %q", err, "boom")
	}
}

func TestValidate_DuplicateUnit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runner := testutil.NewFakeRunner()

	stdout := fmt.Sprintf(`---x.service---
[Unit]
SourcePath=%s

---x.service---
[Unit]
SourcePath=%s

`, filepath.Join(dir, "a.container"), filepath.Join(dir, "b.container"))

	runner.Script(bin, testutil.Result{Stdout: stdout, Stderr: "", Err: nil})

	v := New(bin, runner)

	_, err := v.Validate(t.Context(), dir, true)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want wrapping ErrValidation", err)
	}

	msg := err.Error()
	if !strings.Contains(msg, "same unit") {
		t.Errorf("err.Error() = %q, want to contain %q", msg, "same unit")
	}

	if !strings.Contains(msg, "x.service") {
		t.Errorf("err.Error() = %q, want to contain %q", msg, "x.service")
	}
}

func TestValidate_OrphanUnitFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "orphan.container"), []byte("[Container]\n"), 0o644); err != nil {
		t.Fatalf("write orphan.container: %v", err)
	}

	runner := testutil.NewFakeRunner()
	runner.Script(bin, testutil.Result{Stdout: "", Stderr: "", Err: nil})

	v := New(bin, runner)

	_, err := v.Validate(t.Context(), dir, true)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want wrapping ErrValidation", err)
	}

	if !strings.Contains(err.Error(), "orphan.container") {
		t.Errorf("err.Error() = %q, want to mention %q", err.Error(), "orphan.container")
	}
}

func TestValidate_Warnings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stdout := fmt.Sprintf(`---a.service---
[Unit]
SourcePath=%s

`, filepath.Join(dir, "a.container"))

	const stderr = `quadlet-generator[1]: Loading source unit file /dir/a.container
quadlet-generator[1]: Warning: short image name used
quadlet-generator[1]: processing encountered some errors
`

	runner := testutil.NewFakeRunner()
	runner.Script(bin, testutil.Result{Stdout: stdout, Stderr: stderr, Err: nil})

	v := New(bin, runner)

	report, err := v.Validate(t.Context(), dir, true)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	want := []string{"Warning: short image name used"}
	if !reflect.DeepEqual(report.Warnings, want) {
		t.Errorf("Warnings = %v, want %v", report.Warnings, want)
	}
}

func TestValidate_SymlinkedDir(t *testing.T) {
	t.Parallel()

	realDir := t.TempDir()

	realResolved, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatalf("resolve real dir: %v", err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realResolved, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	stdout := fmt.Sprintf(`---a.service---
[Unit]
SourcePath=%s

[Service]
ExecStart=/usr/bin/podman run a

`, filepath.Join(realResolved, "a.container"))

	runner := testutil.NewFakeRunner()
	runner.Script(bin, testutil.Result{Stdout: stdout, Stderr: "", Err: nil})

	v := New(bin, runner)

	got, err := v.Validate(t.Context(), link, true)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	want := map[string]string{"a.container": "a.service"}
	if !reflect.DeepEqual(got.Units, want) {
		t.Errorf("Units = %v, want %v", got.Units, want)
	}
}

func TestValidate_SourceOutsideDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runner := testutil.NewFakeRunner()

	const stdout = `---x.service---
[Unit]
SourcePath=/elsewhere/x.container

`

	runner.Script(bin, testutil.Result{Stdout: stdout, Stderr: "", Err: nil})

	v := New(bin, runner)

	_, err := v.Validate(t.Context(), dir, true)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}

	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want wrapping ErrValidation", err)
	}
}
