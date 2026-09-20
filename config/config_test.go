// SPDX-License-Identifier: TODO

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetViper clears the global viper state and points HOME and the working
// directory at fresh temp directories so no real ~/.robbe.yaml,
// /etc/robbe/.robbe.yaml or ./.robbe.yaml is picked up.
func resetViper(t *testing.T) string {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	return home
}

//nolint:paralleltest // uses the global viper instance via resetViper
func TestLoad_MissingURL(t *testing.T) {
	resetViper(t)

	Init()

	_, err := Load()
	if !errors.Is(err, ErrRepoURLMissing) {
		t.Fatalf("Load() error = %v, want ErrRepoURLMissing", err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test assumes a non-root user")
	}

	home := resetViper(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("ROBBE_REPO_URL", "https://example.com/repo.git")

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.Ref != DefaultRef {
		t.Errorf("Repo.Ref = %q, want %q", cfg.Repo.Ref, DefaultRef)
	}

	if !cfg.User {
		t.Errorf("User = false, want true")
	}

	wantTarget := filepath.Join(home, ".config", "containers", "systemd", "robbe")
	if cfg.Target != wantTarget {
		t.Errorf("Target = %q, want %q", cfg.Target, wantTarget)
	}

	wantCache := filepath.Join(home, ".cache", "robbe")
	if cfg.Cache != wantCache {
		t.Errorf("Cache = %q, want %q", cfg.Cache, wantCache)
	}

	if cfg.Generator != DefaultGenerator {
		t.Errorf("Generator = %q, want %q", cfg.Generator, DefaultGenerator)
	}

	wantHost, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() error = %v", err)
	}

	if cfg.Host != wantHost {
		t.Errorf("Host = %q, want %q", cfg.Host, wantHost)
	}
}

func TestLoad_XDG(t *testing.T) {
	resetViper(t)

	xdgConfig := t.TempDir()
	xdgCache := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	t.Setenv("ROBBE_REPO_URL", "https://example.com/repo.git")

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantTarget := filepath.Join(xdgConfig, "containers", "systemd", "robbe")
	if cfg.Target != wantTarget {
		t.Errorf("Target = %q, want %q", cfg.Target, wantTarget)
	}

	wantCache := filepath.Join(xdgCache, "robbe")
	if cfg.Cache != wantCache {
		t.Errorf("Cache = %q, want %q", cfg.Cache, wantCache)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	resetViper(t)

	t.Setenv("ROBBE_REPO_URL", "git@example.com:me/repo.git")
	t.Setenv("ROBBE_REPO_REF", "v1")
	t.Setenv("ROBBE_REPO_AUTH_SSH_KEY", "/k")
	t.Setenv("ROBBE_REPO_AUTH_SSH_KEY_PASSWORD", "pw")
	t.Setenv("ROBBE_REPO_AUTH_TOKEN", "tok")
	t.Setenv("ROBBE_REPO_AUTH_USERNAME", "me")
	t.Setenv("ROBBE_HOST", "alpha")
	t.Setenv("ROBBE_TARGET", "/t")
	t.Setenv("ROBBE_CACHE", "/c")
	t.Setenv("ROBBE_GENERATOR", "/g")
	t.Setenv("ROBBE_USER", "false")

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{
		Repo: Repo{
			URL: "git@example.com:me/repo.git",
			Ref: "v1",
			Auth: Auth{
				SSHKey:         "/k",
				SSHKeyPassword: "pw",
				Token:          "tok",
				Username:       "me",
			},
		},
		Host:      "alpha",
		Target:    "/t",
		Cache:     "/c",
		Generator: "/g",
		User:      false,
	}

	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	home := resetViper(t)

	const yamlContent = `
repo:
  url: git@example.com:me/repo.git
  ref: main
  auth:
    ssh_key: /home/user/.ssh/id_ed25519
host: alpha
target: /custom/target
`

	path := filepath.Join(home, ".robbe.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.URL != "git@example.com:me/repo.git" {
		t.Errorf("Repo.URL = %q, want %q", cfg.Repo.URL, "git@example.com:me/repo.git")
	}

	if cfg.Repo.Ref != "main" {
		t.Errorf("Repo.Ref = %q, want %q", cfg.Repo.Ref, "main")
	}

	if cfg.Repo.Auth.SSHKey != "/home/user/.ssh/id_ed25519" {
		t.Errorf("Repo.Auth.SSHKey = %q, want %q", cfg.Repo.Auth.SSHKey, "/home/user/.ssh/id_ed25519")
	}

	if cfg.Host != "alpha" {
		t.Errorf("Host = %q, want %q", cfg.Host, "alpha")
	}

	if cfg.Target != "/custom/target" {
		t.Errorf("Target = %q, want %q", cfg.Target, "/custom/target")
	}

	t.Setenv("ROBBE_REPO_REF", "override")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg2.Repo.Ref != "override" {
		t.Errorf("Repo.Ref = %q, want %q (env override)", cfg2.Repo.Ref, "override")
	}
}
