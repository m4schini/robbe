// SPDX-License-Identifier: TODO

package layout

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFiles creates files under root, given as a map of slash-separated
// relative path to file content. Parent directories are created as needed.
func writeFiles(tb testing.TB, root string, files map[string]string) {
	tb.Helper()

	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			tb.Fatalf("write %s: %v", path, err)
		}
	}
}

func TestResolve_Flat(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"a.container":   "A",
		"sub/b.env":     "B",
		".hidden":       "H",
		".env":          "E",
		".gitignore":    "GI",
		".robbe-commit": "RC",
		".git/config":   "G",
		"k.kube":        "K",
	})

	tree, lay, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Tree{
		"a.container": filepath.Join(repo, "a.container"),
		"sub/b.env":   filepath.Join(repo, "sub", "b.env"),
		".hidden":     filepath.Join(repo, ".hidden"),
		".env":        filepath.Join(repo, ".env"),
	}

	if !reflect.DeepEqual(tree, want) {
		t.Errorf("tree = %#v, want %#v", tree, want)
	}

	if lay.Mode != ModeFlat {
		t.Errorf("Mode = %q, want %q", lay.Mode, ModeFlat)
	}

	if !lay.HostDirFound {
		t.Errorf("HostDirFound = false, want true")
	}

	if !reflect.DeepEqual(lay.Skipped, []string{"k.kube"}) {
		t.Errorf("Skipped = %v, want [k.kube]", lay.Skipped)
	}
}

func TestResolve_HostsAndCommon(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"README.md":               "root file",
		"common/x.network":        "N",
		"common/shared.env":       "S",
		"hosts/alpha/a.container": "A",
		"hosts/beta/b.container":  "B",
	})

	tree, lay, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Tree{
		"x.network":   filepath.Join(repo, "common", "x.network"),
		"shared.env":  filepath.Join(repo, "common", "shared.env"),
		"a.container": filepath.Join(repo, "hosts", "alpha", "a.container"),
	}

	if !reflect.DeepEqual(tree, want) {
		t.Errorf("tree = %#v, want %#v", tree, want)
	}

	if lay.Mode != ModeHosts {
		t.Errorf("Mode = %q, want %q", lay.Mode, ModeHosts)
	}

	if !lay.CommonFound {
		t.Errorf("CommonFound = false, want true")
	}

	if !lay.HostDirFound {
		t.Errorf("HostDirFound = false, want true")
	}

	if _, ok := tree["README.md"]; ok {
		t.Errorf("tree contains README.md, want absent")
	}
}

func TestResolve_HostWins(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"common/n.env":      "common",
		"hosts/alpha/n.env": "host",
	})

	tree, _, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	path, ok := tree["n.env"]
	if !ok {
		t.Fatalf("tree missing n.env")
	}

	wantPath := filepath.Join(repo, "hosts", "alpha", "n.env")
	if path != wantPath {
		t.Errorf("tree[n.env] = %q, want %q", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if string(data) != "host" {
		t.Errorf("content = %q, want %q", string(data), "host")
	}
}

func TestResolve_HostDirMissing(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"hosts/beta/b.container": "B",
		"common/x.network":       "N",
	})

	tree, lay, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Tree{
		"x.network": filepath.Join(repo, "common", "x.network"),
	}

	if !reflect.DeepEqual(tree, want) {
		t.Errorf("tree = %#v, want %#v", tree, want)
	}

	if lay.HostDirFound {
		t.Errorf("HostDirFound = true, want false")
	}

	if !lay.CommonFound {
		t.Errorf("CommonFound = false, want true")
	}
}

func TestResolve_HostsWithoutCommon(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"hosts/beta/b.container": "B",
	})

	tree, lay, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(tree) != 0 {
		t.Errorf("tree = %#v, want empty", tree)
	}

	if lay.HostDirFound {
		t.Errorf("HostDirFound = true, want false")
	}

	if lay.CommonFound {
		t.Errorf("CommonFound = true, want false")
	}
}

func TestResolve_SkippedKubeInHosts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"hosts/alpha/k.kube": "K",
	})

	tree, lay, err := Resolve(repo, "alpha")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !reflect.DeepEqual(lay.Skipped, []string{"k.kube"}) {
		t.Errorf("Skipped = %v, want [k.kube]", lay.Skipped)
	}

	if _, ok := tree["k.kube"]; ok {
		t.Errorf("tree contains k.kube, want absent")
	}
}

func TestTree_Paths(t *testing.T) {
	t.Parallel()

	tree := Tree{
		"z.container": "/z",
		"a.container": "/a",
		"m.container": "/m",
	}

	want := []string{"a.container", "m.container", "z.container"}

	if got := tree.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
}
