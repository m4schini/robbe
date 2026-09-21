// SPDX-License-Identifier: TODO

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const repoURL = "https://example.com/repo.git"

// resetViper clears the global viper state and ConfigFile, points HOME and
// the working directory at fresh temp directories and clears every XDG
// variable, so no real config file is picked up. It returns the home dir.
func resetViper(t *testing.T) string {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	ConfigFile = ""

	t.Cleanup(func() { ConfigFile = "" })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_CONFIG_DIRS", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}

	return home
}

// writeConfig writes content to dir/config.yaml, creating dir.
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
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
	t.Setenv("ROBBE_REPO_URL", repoURL)

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

	wantState := filepath.Join(home, ".local", "state", "robbe")
	if cfg.State != wantState {
		t.Errorf("State = %q, want %q", cfg.State, wantState)
	}

	if cfg.Runtime != "" {
		t.Errorf("Runtime = %q, want empty (XDG_RUNTIME_DIR unset)", cfg.Runtime)
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
	if os.Geteuid() == 0 {
		t.Skip("test assumes a non-root user")
	}

	resetViper(t)

	xdgConfig := t.TempDir()
	xdgCache := t.TempDir()
	xdgState := t.TempDir()
	xdgRuntime := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	t.Setenv("XDG_STATE_HOME", xdgState)
	t.Setenv("XDG_RUNTIME_DIR", xdgRuntime)
	t.Setenv("ROBBE_REPO_URL", repoURL)

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

	wantState := filepath.Join(xdgState, "robbe")
	if cfg.State != wantState {
		t.Errorf("State = %q, want %q", cfg.State, wantState)
	}

	wantRuntime := filepath.Join(xdgRuntime, "robbe")
	if cfg.Runtime != wantRuntime {
		t.Errorf("Runtime = %q, want %q", cfg.Runtime, wantRuntime)
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
	t.Setenv("ROBBE_STATE", "/s")
	t.Setenv("ROBBE_RUNTIME", "/r")
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
		State:     "/s",
		Runtime:   "/r",
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
state: /custom/state
runtime: /custom/runtime
`

	writeConfig(t, filepath.Join(home, ".config", "robbe"), yamlContent)

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

	if cfg.State != "/custom/state" {
		t.Errorf("State = %q, want %q", cfg.State, "/custom/state")
	}

	if cfg.Runtime != "/custom/runtime" {
		t.Errorf("Runtime = %q, want %q", cfg.Runtime, "/custom/runtime")
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

//nolint:paralleltest // t.Setenv forbids t.Parallel
func TestSearchDirs(t *testing.T) {
	xdgConfig := t.TempDir()
	first := t.TempDir()
	second := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CONFIG_DIRS", first+":"+second)

	home := t.TempDir()
	etc := filepath.Join(systemConfigRoot, "robbe")

	tests := []struct {
		name string
		user bool
		want []string
	}{
		{
			name: "user honours XDG_CONFIG_HOME, XDG_CONFIG_DIRS, then /etc",
			user: true,
			want: []string{
				filepath.Join(xdgConfig, "robbe"),
				filepath.Join(first, "robbe"),
				filepath.Join(second, "robbe"),
				etc,
			},
		},
		{
			name: "root ignores XDG variables and reads only /etc",
			user: false,
			want: []string{etc},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchDirs(tt.user, home)
			if strings.Join(got, ":") != strings.Join(tt.want, ":") {
				t.Errorf("searchDirs(%v) = %q, want %q", tt.user, got, tt.want)
			}
		})
	}
}

func TestInit_ConfigHomeWinsOverConfigDirs(t *testing.T) {
	resetViper(t)

	xdgConfig := t.TempDir()
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CONFIG_DIRS", xdgDir)

	writeConfig(t, filepath.Join(xdgConfig, "robbe"), "repo:\n  url: https://example.com/home.git\n")
	writeConfig(t, filepath.Join(xdgDir, "robbe"), "repo:\n  url: https://example.com/dirs.git\n")

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.URL != "https://example.com/home.git" {
		t.Errorf("Repo.URL = %q, want the XDG_CONFIG_HOME file to win", cfg.Repo.URL)
	}
}

func TestInit_ConfigDirs(t *testing.T) {
	resetViper(t)

	first := t.TempDir()
	second := t.TempDir()
	t.Setenv("XDG_CONFIG_DIRS", first+":"+second)

	// Only the second entry holds a file; the first is searched and skipped.
	writeConfig(t, filepath.Join(second, "robbe"), "repo:\n  url: "+repoURL+"\n")

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.URL != repoURL {
		t.Errorf("Repo.URL = %q, want %q (from XDG_CONFIG_DIRS)", cfg.Repo.URL, repoURL)
	}
}

//nolint:paralleltest // uses the global viper instance via resetViper
func TestInit_LegacyDotfileIgnored(t *testing.T) {
	home := resetViper(t)

	const legacy = "repo:\n  url: " + repoURL + "\n"

	for _, path := range []string{filepath.Join(home, ".robbe.yaml"), ".robbe.yaml"} {
		if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	Init()

	_, err := Load()
	if !errors.Is(err, ErrRepoURLMissing) {
		t.Fatalf("Load() error = %v, want ErrRepoURLMissing (legacy dotfiles must not be read)", err)
	}
}

//nolint:paralleltest // uses the global viper instance via resetViper
func TestInit_ConfigFlag(t *testing.T) {
	home := resetViper(t)

	// A file in the search path that must lose against --config.
	writeConfig(t, filepath.Join(home, ".config", "robbe"), "repo:\n  url: https://example.com/search.git\n")

	explicit := filepath.Join(t.TempDir(), "elsewhere.yaml")
	if err := os.WriteFile(explicit, []byte("repo:\n  url: https://example.com/flag.git\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", explicit, err)
	}

	ConfigFile = explicit

	Init()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.URL != "https://example.com/flag.git" {
		t.Errorf("Repo.URL = %q, want the --config file to win", cfg.Repo.URL)
	}
}

//nolint:paralleltest // uses the global viper instance via resetViper
func TestInit_ConfigFlagMissing(t *testing.T) {
	resetViper(t)

	ConfigFile = filepath.Join(t.TempDir(), "nope.yaml")

	err := configure()
	if err == nil {
		t.Fatal("configure() error = nil, want error for a missing --config file")
	}

	if !strings.Contains(err.Error(), "read config "+ConfigFile) {
		t.Errorf("configure() error = %q, want to name %s", err, ConfigFile)
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("configure() error = %v, want to wrap os.ErrNotExist", err)
	}
}

// TestConfigure_NoHome covers the system unit, which runs without $HOME
// (systemd.exec(5) sets it only for units with User=). Root must not need it;
// a non-root user still does.
func TestConfigure_NoHome(t *testing.T) {
	resetViper(t)
	t.Setenv("HOME", "")

	err := configure()

	if os.Geteuid() == 0 {
		if err != nil {
			t.Fatalf("configure() error = %v, want nil for root without $HOME", err)
		}

		return
	}

	if err == nil {
		t.Fatal("configure() error = nil, want error for a non-root user without $HOME")
	}

	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("configure() error = %q, want to mention the home directory", err)
	}
}

//nolint:paralleltest // uses the global viper instance via resetViper
func TestInit_MalformedYAML(t *testing.T) {
	home := resetViper(t)

	path := writeConfig(t, filepath.Join(home, ".config", "robbe"), "key: [\n")

	err := configure()
	if err == nil {
		t.Fatal("configure() error = nil, want error for malformed YAML in the search path")
	}

	if !strings.Contains(err.Error(), "read config "+path) {
		t.Errorf("configure() error = %q, want to name %s", err, path)
	}
}
