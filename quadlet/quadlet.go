// SPDX-License-Identifier: TODO

// Package quadlet holds the naming rules shared by the planner and the
// generator adapter.
package quadlet

import (
	"path/filepath"
	"strings"
)

// unitSuffix maps a quadlet file extension to the suffix podman appends to
// the service name when no ServiceName= is set.
var unitSuffix = map[string]string{
	".container": "",
	".pod":       "-pod",
	".volume":    "-volume",
	".network":   "-network",
	".image":     "-image",
	".build":     "-build",
	".kube":      "",
}

// IsUnitFile reports whether rel names a quadlet unit file.
func IsUnitFile(rel string) bool {
	_, ok := unitSuffix[filepath.Ext(rel)]

	return ok
}

// IsWorkload reports whether rel is a .container or .pod file: the units
// that read env and config files and are restarted when a support file
// changes.
func IsWorkload(rel string) bool {
	ext := filepath.Ext(rel)

	return ext == ".container" || ext == ".pod"
}

// ConventionalUnit derives the service name podman would generate for rel
// without a ServiceName= override.
func ConventionalUnit(rel string) string {
	base := filepath.Base(rel)
	ext := filepath.Ext(base)

	return strings.TrimSuffix(base, ext) + unitSuffix[ext] + ".service"
}
