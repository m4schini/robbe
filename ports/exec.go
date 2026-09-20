// SPDX-License-Identifier: TODO

package ports

import "context"

// Runner executes external commands. It is the single seam through which
// robbe reaches systemctl and the quadlet generator, so tests can inject
// scripted results instead of shimming PATH.
type Runner interface {
	// Run executes name with args. env entries (KEY=VALUE) are added to the
	// inherited environment. It returns the captured stdout and stderr; err
	// is non-nil when the command could not be started or exited non-zero.
	Run(ctx context.Context, name string, env []string, args ...string) (stdout, stderr []byte, err error)
}
