// SPDX-License-Identifier: TODO

package ports

import "context"

// Report is the outcome of a generator dry-run.
type Report struct {
	// Units maps each quadlet file (slash path relative to the validated
	// dir) to the systemd unit it generates.
	Units map[string]string
	// Warnings are generator messages that did not fail the run.
	Warnings []string
}

// Validator checks a directory of quadlet files with the quadlet generator.
type Validator interface {
	// Validate runs the generator in dry-run mode over dir. A non-nil error
	// means at least one file was rejected; the returned Report then still
	// lists the units of the files the generator accepted.
	Validate(ctx context.Context, dir string, user bool) (Report, error)
}
