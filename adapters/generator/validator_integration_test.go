//go:build integration

// SPDX-License-Identifier: TODO

package generator

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m4schini/robbe/adapters/osexec"
)

const generatorBin = "/usr/lib/systemd/system-generators/podman-system-generator"

func skipUnlessGeneratorPresent(tb testing.TB) {
	tb.Helper()

	if _, err := os.Stat(generatorBin); err != nil {
		tb.Skip("podman-system-generator not present at " + generatorBin)
	}
}

func TestValidate_Integration_ParsesUnits(t *testing.T) {
	t.Parallel()

	skipUnlessGeneratorPresent(t)

	dir := t.TempDir()

	writeFixture(t, dir, "nginx.container", "[Container]\nImage=docker.io/library/nginx\n")
	writeFixture(t, dir, filepath.Join("sub", "redis.container"), "[Container]\nImage=docker.io/library/redis\nServiceName=cache\n")
	writeFixture(t, dir, "proxy.network", "[Network]\n")

	v := New(generatorBin, osexec.New())

	got, err := v.Validate(t.Context(), dir, true)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	want := map[string]string{
		"nginx.container":     "nginx.service",
		"sub/redis.container": "cache.service",
		"proxy.network":       "proxy-network.service",
	}

	if !reflect.DeepEqual(got.Units, want) {
		t.Errorf("Validate() = %v, want %v", got.Units, want)
	}
}

func TestValidate_Integration_Error(t *testing.T) {
	t.Parallel()

	skipUnlessGeneratorPresent(t)

	dir := t.TempDir()

	writeFixture(t, dir, "bad.container", "[Container]\nImg=x\n")

	v := New(generatorBin, osexec.New())

	_, err := v.Validate(t.Context(), dir, true)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want wrapping ErrValidation", err)
	}

	if !strings.Contains(err.Error(), "bad.container") {
		t.Errorf("err = %q, want to mention %q", err.Error(), "bad.container")
	}
}

// writeFixture writes content to a file at rel under dir, creating parent
// directories as needed.
func writeFixture(tb testing.TB, dir, rel, content string) {
	tb.Helper()

	path := filepath.Join(dir, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}
