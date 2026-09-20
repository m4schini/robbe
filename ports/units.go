// SPDX-License-Identifier: TODO

package ports

import "context"

// UnitManager drives systemd. The scope (user or system) is fixed when the
// implementation is constructed.
type UnitManager interface {
	// DaemonReload re-runs the generators and reloads unit definitions.
	DaemonReload(ctx context.Context) error
	// Start starts the given units in one transaction. No-op for empty input.
	Start(ctx context.Context, units ...string) error
	// Restart restarts (or starts) the given units in one transaction.
	Restart(ctx context.Context, units ...string) error
	// Stop stops the given units in one transaction.
	Stop(ctx context.Context, units ...string) error
	// ActiveStates returns the ActiveState of each unit ("active",
	// "inactive", "failed", ...). Unknown units report "inactive".
	ActiveStates(ctx context.Context, units ...string) (map[string]string, error)
}
