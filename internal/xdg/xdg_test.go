// SPDX-License-Identifier: TODO

package xdg

import (
	"path/filepath"
	"reflect"
	"testing"
)

const home = "/home/x"

// envCase is one value of an XDG variable and the directory it resolves to.
type envCase struct {
	name  string
	value string
	want  string
}

// homeCases returns the four shared cases for a *_HOME variable whose
// fallback is home/<fallback>.
func homeCases(fallback string) []envCase {
	return []envCase{
		{name: "unset", value: "", want: filepath.Join(home, fallback)},
		{name: "relative", value: "rel/dir", want: filepath.Join(home, fallback)},
		{name: "absolute", value: "/xdg", want: "/xdg"},
		{name: "absolute unclean", value: "/xdg//dir/", want: "/xdg/dir"},
	}
}

//nolint:paralleltest // t.Setenv inside the table loop
func TestConfigHome(t *testing.T) {
	for _, tt := range homeCases(".config") {
		t.Setenv(envConfigHome, tt.value)

		if got := ConfigHome(home); got != tt.want {
			t.Errorf("%s: ConfigHome() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

//nolint:paralleltest // t.Setenv inside the table loop
func TestCacheHome(t *testing.T) {
	for _, tt := range homeCases(".cache") {
		t.Setenv(envCacheHome, tt.value)

		if got := CacheHome(home); got != tt.want {
			t.Errorf("%s: CacheHome() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

//nolint:paralleltest // t.Setenv inside the table loop
func TestStateHome(t *testing.T) {
	for _, tt := range homeCases(filepath.Join(".local", "state")) {
		t.Setenv(envStateHome, tt.value)

		if got := StateHome(home); got != tt.want {
			t.Errorf("%s: StateHome() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

//nolint:paralleltest // t.Setenv inside the table loop
func TestRuntimeDir(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{name: "unset", value: "", want: "", ok: false},
		{name: "relative", value: "run/user", want: "", ok: false},
		{name: "absolute", value: "/run/user/1000", want: "/run/user/1000", ok: true},
		{name: "absolute unclean", value: "/run/user/1000/", want: "/run/user/1000", ok: true},
	}

	for _, tt := range tests {
		t.Setenv(envRuntimeDir, tt.value)

		got, ok := RuntimeDir()
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: RuntimeDir() = (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

//nolint:paralleltest // t.Setenv inside the table loop
func TestConfigDirs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "unset", value: "", want: []string{"/etc/xdg"}},
		{name: "only relative", value: "a:b/c", want: []string{"/etc/xdg"}},
		{name: "mixed", value: "a:/b::c/d", want: []string{"/b"}},
		{name: "two absolute", value: "/one:/two", want: []string{"/one", "/two"}},
	}

	for _, tt := range tests {
		t.Setenv(envConfigDirs, tt.value)

		if got := ConfigDirs(); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: ConfigDirs() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDefaults_User(t *testing.T) {
	t.Setenv(envConfigHome, "/c")
	t.Setenv(envCacheHome, "/k")
	t.Setenv(envStateHome, "/s")
	t.Setenv(envRuntimeDir, "/r")

	want := Paths{
		Config:     "/c/robbe",
		Cache:      "/k/robbe",
		State:      "/s/robbe",
		Runtime:    "/r/robbe",
		Target:     "/c/containers/systemd/robbe",
		RuntimeSet: true,
	}

	if got := Defaults("robbe", true, home); got != want {
		t.Errorf("Defaults(user) = %+v, want %+v", got, want)
	}
}

func TestDefaults_UserFallbacks(t *testing.T) {
	t.Setenv(envConfigHome, "")
	t.Setenv(envCacheHome, "")
	t.Setenv(envStateHome, "")
	t.Setenv(envRuntimeDir, "")

	want := Paths{
		Config:     filepath.Join(home, ".config", "robbe"),
		Cache:      filepath.Join(home, ".cache", "robbe"),
		State:      filepath.Join(home, ".local", "state", "robbe"),
		Runtime:    "",
		Target:     filepath.Join(home, ".config", "containers", "systemd", "robbe"),
		RuntimeSet: false,
	}

	if got := Defaults("robbe", true, home); got != want {
		t.Errorf("Defaults(user, no XDG) = %+v, want %+v", got, want)
	}
}

func TestDefaults_Root(t *testing.T) {
	// Root ignores the XDG variables entirely.
	t.Setenv(envConfigHome, "/c")
	t.Setenv(envCacheHome, "/k")
	t.Setenv(envStateHome, "/s")
	t.Setenv(envRuntimeDir, "/r")

	want := Paths{
		Config:     "/etc/robbe",
		Cache:      "/var/cache/robbe",
		State:      "/var/lib/robbe",
		Runtime:    "/run/robbe",
		Target:     "/etc/containers/systemd/robbe",
		RuntimeSet: true,
	}

	if got := Defaults("robbe", false, "/root"); got != want {
		t.Errorf("Defaults(root) = %+v, want %+v", got, want)
	}
}
