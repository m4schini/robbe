// SPDX-License-Identifier: TODO

package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m4schini/robbe/app/layout"
	"github.com/m4schini/robbe/app/marker"
)

// writeTree writes files (slash-separated relative path -> content) under
// dir and returns a layout.Tree pointing at them.
func writeTree(tb testing.TB, dir string, files map[string]string) layout.Tree {
	tb.Helper()

	tree := layout.Tree{}

	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			tb.Fatalf("write %s: %v", path, err)
		}

		tree[rel] = path
	}

	return tree
}

// writeTarget writes files (slash-separated relative path -> content)
// directly under dir, simulating the current state of the target directory.
func writeTarget(tb testing.TB, dir string, files map[string]string) {
	tb.Helper()

	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			tb.Fatalf("write %s: %v", path, err)
		}
	}
}

func TestDiff_AddChangeRemoveUnchanged(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"nginx.container": "A",
		"proxy.network":   "N",
		"same.env":        "S",
	})
	writeTarget(t, targetDir, map[string]string{
		"nginx.container": "OLD",
		"db.container":    "D",
		"same.env":        "S",
	})

	desiredUnits := map[string]string{
		"nginx.container": "nginx.service",
		"proxy.network":   "proxy-network.service",
	}
	targetUnits := map[string]string{
		"db.container":    "db.service",
		"nginx.container": "nginx.service",
	}

	p, err := Diff(desired, targetDir, desiredUnits, targetUnits)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantAdd := []File{{Rel: "proxy.network", Unit: "proxy-network.service"}}
	wantChange := []File{{Rel: "nginx.container", Unit: "nginx.service"}}
	wantRemove := []File{{Rel: "db.container", Unit: "db.service"}}

	if !reflect.DeepEqual(p.Add, wantAdd) {
		t.Errorf("Add = %+v, want %+v", p.Add, wantAdd)
	}

	if !reflect.DeepEqual(p.Change, wantChange) {
		t.Errorf("Change = %+v, want %+v", p.Change, wantChange)
	}

	if !reflect.DeepEqual(p.Remove, wantRemove) {
		t.Errorf("Remove = %+v, want %+v", p.Remove, wantRemove)
	}

	if !reflect.DeepEqual(p.Start, []string{"proxy-network.service"}) {
		t.Errorf("Start = %v, want [proxy-network.service]", p.Start)
	}

	if !reflect.DeepEqual(p.Restart, []string{"nginx.service"}) {
		t.Errorf("Restart = %v, want [nginx.service]", p.Restart)
	}

	if !reflect.DeepEqual(p.Stop, []string{"db.service"}) {
		t.Errorf("Stop = %v, want [db.service]", p.Stop)
	}

	if p.RestartAll {
		t.Errorf("RestartAll = true, want false")
	}

	if p.Empty() {
		t.Errorf("Empty() = true, want false")
	}
}

func TestDiff_SupportFileChangeRestartsAll(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"a.container": "A",
		"b.container": "B",
		"app.env":     "v2",
	})
	writeTarget(t, targetDir, map[string]string{
		"a.container": "A",
		"b.container": "B",
		"app.env":     "v1",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantChange := []File{{Rel: "app.env", Unit: ""}}
	if !reflect.DeepEqual(p.Change, wantChange) {
		t.Errorf("Change = %+v, want %+v", p.Change, wantChange)
	}

	if !p.RestartAll {
		t.Errorf("RestartAll = false, want true")
	}

	if !reflect.DeepEqual(p.Restart, []string{"a.service", "b.service"}) {
		t.Errorf("Restart = %v, want [a.service b.service]", p.Restart)
	}

	if len(p.Start) != 0 {
		t.Errorf("Start = %v, want empty", p.Start)
	}
}

func TestDiff_SupportFileAddedRestartsAllExceptStarted(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"a.container":   "A",
		"new.container": "N",
		"app.env":       "v1",
	})
	writeTarget(t, targetDir, map[string]string{
		"a.container": "A",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if !reflect.DeepEqual(p.Start, []string{"new.service"}) {
		t.Errorf("Start = %v, want [new.service]", p.Start)
	}

	if !reflect.DeepEqual(p.Restart, []string{"a.service"}) {
		t.Errorf("Restart = %v, want [a.service]", p.Restart)
	}

	if !p.RestartAll {
		t.Errorf("RestartAll = false, want true")
	}
}

func TestDiff_RemovedSupportFileRestartsAll(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"a.container": "A",
	})
	writeTarget(t, targetDir, map[string]string{
		"a.container": "A",
		"old.env":     "old",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantRemove := []File{{Rel: "old.env", Unit: ""}}
	if !reflect.DeepEqual(p.Remove, wantRemove) {
		t.Errorf("Remove = %+v, want %+v", p.Remove, wantRemove)
	}

	if !p.RestartAll {
		t.Errorf("RestartAll = false, want true")
	}

	if !reflect.DeepEqual(p.Restart, []string{"a.service"}) {
		t.Errorf("Restart = %v, want [a.service]", p.Restart)
	}

	if len(p.Start) != 0 {
		t.Errorf("Start = %v, want empty", p.Start)
	}

	if len(p.Stop) != 0 {
		t.Errorf("Stop = %v, want empty", p.Stop)
	}
}

func TestDiff_MarkerIgnored(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"a.container": "A",
	})
	writeTarget(t, targetDir, map[string]string{
		"a.container": "A",
		marker.File:   "commit\n2024-01-01T00:00:00Z\n",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if !p.Empty() {
		t.Errorf("Empty() = false, want true (plan = %+v)", p)
	}
}

func TestDiff_MissingTargetDir(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "missing")

	desired := writeTree(t, srcDir, map[string]string{
		"a.container": "A",
		"b.env":       "B",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantAdd := []File{
		{Rel: "a.container", Unit: "a.service"},
		{Rel: "b.env", Unit: ""},
	}

	if !reflect.DeepEqual(p.Add, wantAdd) {
		t.Errorf("Add = %+v, want %+v", p.Add, wantAdd)
	}

	if len(p.Change) != 0 || len(p.Remove) != 0 {
		t.Errorf("Change/Remove not empty: %+v / %+v", p.Change, p.Remove)
	}
}

func TestDiff_FallbackUnitNames(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"x.container": "1",
		"x.pod":       "2",
		"x.volume":    "3",
		"x.network":   "4",
		"x.image":     "5",
		"x.build":     "6",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	got := map[string]string{}
	for _, f := range p.Add {
		got[f.Rel] = f.Unit
	}

	want := map[string]string{
		"x.container": "x.service",
		"x.pod":       "x-pod.service",
		"x.volume":    "x-volume.service",
		"x.network":   "x-network.service",
		"x.image":     "x-image.service",
		"x.build":     "x-build.service",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("units = %+v, want %+v", got, want)
	}
}

func TestDiff_NestedPaths(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"sub/dir/c.container": "C",
	})
	writeTarget(t, targetDir, map[string]string{
		"sub/old.env": "old",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantAdd := []File{{Rel: "sub/dir/c.container", Unit: "c.service"}}
	if !reflect.DeepEqual(p.Add, wantAdd) {
		t.Errorf("Add = %+v, want %+v", p.Add, wantAdd)
	}

	wantRemove := []File{{Rel: "sub/old.env", Unit: ""}}
	if !reflect.DeepEqual(p.Remove, wantRemove) {
		t.Errorf("Remove = %+v, want %+v", p.Remove, wantRemove)
	}
}

func TestDiff_Deterministic(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"nginx.container": "A",
		"proxy.network":   "N",
		"app.env":         "v2",
	})
	writeTarget(t, targetDir, map[string]string{
		"nginx.container": "OLD",
		"db.container":    "D",
		"app.env":         "v1",
	})

	p1, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	p2, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if !reflect.DeepEqual(p1, p2) {
		t.Errorf("Diff() not deterministic: %+v vs %+v", p1, p2)
	}

	for _, files := range [][]File{p1.Add, p1.Change, p1.Remove} {
		for i := 1; i < len(files); i++ {
			if files[i-1].Rel >= files[i].Rel {
				t.Errorf("files not sorted by Rel: %+v", files)
			}
		}
	}
}

func TestPlan_String(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"nginx.container": "NEW",
		"proxy.network":   "N",
		"app.env":         "V2",
	})
	writeTarget(t, targetDir, map[string]string{
		"nginx.container": "OLD",
		"db.container":    "D",
	})

	desiredUnits := map[string]string{
		"nginx.container": "nginx.service",
		"proxy.network":   "proxy-network.service",
	}
	targetUnits := map[string]string{
		"db.container":    "db.service",
		"nginx.container": "nginx.service",
	}

	p, err := Diff(desired, targetDir, desiredUnits, targetUnits)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	out := p.String()

	for _, want := range []string{
		"  + proxy.network",
		"  ~ nginx.container",
		"  - db.container",
		"  stop     db.service",
		"  start    proxy-network.service",
		"  restart  nginx.service",
		"(support file -> restart all)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("String() missing %q, got:\n%s", want, out)
		}
	}

	var zero Plan

	empty := zero.String()
	if got := strings.Count(empty, "(none)"); got != 2 {
		t.Errorf("empty Plan String() has %d (none), want 2:\n%s", got, empty)
	}
}

func TestDiff_RenamedServiceName(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"y.container": "SAME",
	})
	writeTarget(t, targetDir, map[string]string{
		"x.container": "SAME",
	})

	desiredUnits := map[string]string{"y.container": "x.service"}
	targetUnits := map[string]string{"x.container": "x.service"}

	p, err := Diff(desired, targetDir, desiredUnits, targetUnits)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantAdd := []File{{Rel: "y.container", Unit: "x.service"}}
	wantRemove := []File{{Rel: "x.container", Unit: "x.service"}}

	if !reflect.DeepEqual(p.Add, wantAdd) {
		t.Errorf("Add = %+v, want %+v", p.Add, wantAdd)
	}

	if !reflect.DeepEqual(p.Remove, wantRemove) {
		t.Errorf("Remove = %+v, want %+v", p.Remove, wantRemove)
	}

	if len(p.Start) != 0 {
		t.Errorf("Start = %v, want empty", p.Start)
	}

	if len(p.Stop) != 0 {
		t.Errorf("Stop = %v, want empty", p.Stop)
	}

	if len(p.Restart) != 0 {
		t.Errorf("Restart = %v, want empty", p.Restart)
	}
}

func TestDiff_ServiceNameChanged(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"x.container": "NEW",
	})
	writeTarget(t, targetDir, map[string]string{
		"x.container": "OLD",
	})

	desiredUnits := map[string]string{"x.container": "new.service"}
	targetUnits := map[string]string{"x.container": "old.service"}

	p, err := Diff(desired, targetDir, desiredUnits, targetUnits)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if !reflect.DeepEqual(p.Start, []string{"new.service"}) {
		t.Errorf("Start = %v, want [new.service]", p.Start)
	}

	if !reflect.DeepEqual(p.Stop, []string{"old.service"}) {
		t.Errorf("Stop = %v, want [old.service]", p.Stop)
	}

	if len(p.Restart) != 0 {
		t.Errorf("Restart = %v, want empty", p.Restart)
	}
}

func TestDiff_MovedFile(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"b/x.container": "SAME",
	})
	writeTarget(t, targetDir, map[string]string{
		"a/x.container": "SAME",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	wantAdd := []File{{Rel: "b/x.container", Unit: "x.service"}}
	wantRemove := []File{{Rel: "a/x.container", Unit: "x.service"}}

	if !reflect.DeepEqual(p.Add, wantAdd) {
		t.Errorf("Add = %+v, want %+v", p.Add, wantAdd)
	}

	if !reflect.DeepEqual(p.Remove, wantRemove) {
		t.Errorf("Remove = %+v, want %+v", p.Remove, wantRemove)
	}

	if len(p.Start) != 0 || len(p.Stop) != 0 || len(p.Restart) != 0 {
		t.Errorf("unit actions not empty: start=%v stop=%v restart=%v", p.Start, p.Stop, p.Restart)
	}
}

func TestDiff_RestartAllOnlyWorkloads(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desiredFiles := map[string]string{
		"a.container": "A",
		"p.pod":       "P",
		"n.network":   "N",
		"v.volume":    "V",
		"i.image":     "I",
		"app.env":     "v2",
	}
	targetFiles := map[string]string{
		"a.container": "A",
		"p.pod":       "P",
		"n.network":   "N",
		"v.volume":    "V",
		"i.image":     "I",
		"app.env":     "v1",
	}

	desired := writeTree(t, srcDir, desiredFiles)
	writeTarget(t, targetDir, targetFiles)

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if !p.RestartAll {
		t.Fatalf("RestartAll = false, want true")
	}

	want := []string{"a.service", "p-pod.service"}
	if !reflect.DeepEqual(p.Restart, want) {
		t.Errorf("Restart = %v, want %v", p.Restart, want)
	}
}

func TestDiff_StaleTempIgnored(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	targetDir := t.TempDir()

	desired := writeTree(t, srcDir, map[string]string{
		"a.container": "A",
	})
	writeTarget(t, targetDir, map[string]string{
		"a.container":       "A",
		".robbe-tmp-abc123": "junk",
	})

	p, err := Diff(desired, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if len(p.Remove) != 0 {
		t.Errorf("Remove = %+v, want empty", p.Remove)
	}

	if p.RestartAll {
		t.Errorf("RestartAll = true, want false")
	}

	if !p.Empty() {
		t.Errorf("Empty() = false, want true (plan = %+v)", p)
	}
}
