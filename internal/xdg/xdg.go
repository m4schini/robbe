// SPDX-License-Identifier: TODO

// Package xdg resolves the directories of the XDG Base Directory
// Specification (version 0.8) and switches to the system-wide equivalents
// (/etc, /var/cache, /var/lib, /run) when robbe runs as root.
//
// Every lookup applies the same rule the specification prescribes: an
// environment variable that is unset, empty or holds a relative path is
// ignored and the documented fallback applies.
package xdg

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	envConfigHome = "XDG_CONFIG_HOME"
	envCacheHome  = "XDG_CACHE_HOME"
	envStateHome  = "XDG_STATE_HOME"
	envRuntimeDir = "XDG_RUNTIME_DIR"
	envConfigDirs = "XDG_CONFIG_DIRS"

	// defaultConfigDirs is the spec default for XDG_CONFIG_DIRS.
	defaultConfigDirs = "/etc/xdg"
)

// Paths are the directories one application owns, resolved for the
// current scope.
type Paths struct {
	// Config is the directory holding the application's config file.
	Config string
	// Cache holds data the application can regenerate at any time.
	Cache string
	// State holds data that persists between runs but is not configuration.
	State string
	// Runtime holds per-boot files such as locks and sockets. Empty when
	// RuntimeSet is false.
	Runtime string
	// Target is the podman quadlet directory of the application.
	Target string
	// RuntimeSet reports whether Runtime could be resolved. It is false for
	// a user without XDG_RUNTIME_DIR; the spec leaves the fallback to the
	// application.
	RuntimeSet bool
}

// Defaults resolves the directories of app. For root (user == false) the
// system-wide locations are used; otherwise the XDG variables with their
// spec fallbacks below home apply.
func Defaults(app string, user bool, home string) Paths {
	if !user {
		return Paths{
			Config:     "/etc/" + app,
			Cache:      "/var/cache/" + app,
			State:      "/var/lib/" + app,
			Runtime:    "/run/" + app,
			Target:     "/etc/containers/systemd/" + app,
			RuntimeSet: true,
		}
	}

	paths := Paths{
		Config:     filepath.Join(ConfigHome(home), app),
		Cache:      filepath.Join(CacheHome(home), app),
		State:      filepath.Join(StateHome(home), app),
		Runtime:    "",
		Target:     filepath.Join(ConfigHome(home), "containers", "systemd", app),
		RuntimeSet: false,
	}

	if runtime, ok := RuntimeDir(); ok {
		paths.Runtime = filepath.Join(runtime, app)
		paths.RuntimeSet = true
	}

	return paths
}

// ConfigHome returns $XDG_CONFIG_HOME, or home/.config when the variable is
// unset, empty or relative.
func ConfigHome(home string) string {
	if dir, ok := absolute(os.Getenv(envConfigHome)); ok {
		return dir
	}

	return filepath.Join(home, ".config")
}

// CacheHome returns $XDG_CACHE_HOME, or home/.cache when the variable is
// unset, empty or relative.
func CacheHome(home string) string {
	if dir, ok := absolute(os.Getenv(envCacheHome)); ok {
		return dir
	}

	return filepath.Join(home, ".cache")
}

// StateHome returns $XDG_STATE_HOME, or home/.local/state when the variable
// is unset, empty or relative.
func StateHome(home string) string {
	if dir, ok := absolute(os.Getenv(envStateHome)); ok {
		return dir
	}

	return filepath.Join(home, ".local", "state")
}

// RuntimeDir returns $XDG_RUNTIME_DIR and true, or "" and false when the
// variable is unset, empty or relative. The specification defines no
// fallback; the caller decides.
func RuntimeDir() (string, bool) {
	return absolute(os.Getenv(envRuntimeDir))
}

// ConfigDirs returns the entries of $XDG_CONFIG_DIRS in order of
// preference, dropping empty and relative entries. When nothing remains the
// spec default [/etc/xdg] is returned.
func ConfigDirs() []string {
	var dirs []string

	for entry := range strings.SplitSeq(os.Getenv(envConfigDirs), ":") {
		if dir, ok := absolute(entry); ok {
			dirs = append(dirs, dir)
		}
	}

	if len(dirs) == 0 {
		return []string{defaultConfigDirs}
	}

	return dirs
}

// absolute applies the spec rule for every XDG variable: a value counts
// only when it is non-empty and absolute. The returned path is cleaned.
func absolute(value string) (string, bool) {
	if value == "" || !filepath.IsAbs(value) {
		return "", false
	}

	return filepath.Clean(value), true
}
