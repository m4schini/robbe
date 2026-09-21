// SPDX-License-Identifier: TODO

// Package status reports the applied commit, the remote head and the drift
// between the target directory and the repository.
package status

import (
	"context"
	"fmt"

	"github.com/m4schini/robbe/app/layout"
	"github.com/m4schini/robbe/app/marker"
	"github.com/m4schini/robbe/app/plan"
	"github.com/m4schini/robbe/ports"
)

// Options are the parts of the configuration status needs.
type Options struct {
	URL    string
	Ref    string
	Auth   ports.Auth
	Host   string
	Target string
	// State is the directory holding the applied-commit marker.
	State string
}

// Status is the result of Get.
type Status struct {
	// Applied is the marker in the state dir; zero when nothing was applied yet.
	Applied marker.Marker
	// Remote is the commit ref points to on the remote.
	Remote string
	Layout layout.Layout
	// Drift lists what a sync of Remote would change. Unit names in it are
	// derived by convention, not by the generator.
	Drift plan.Plan
}

// Get reads the marker, fetches ref and diffs the desired tree of the
// fetched commit against the target.
func Get(ctx context.Context, src ports.Source, opts Options) (Status, error) {
	applied, err := marker.Read(opts.State)
	if err != nil {
		return Status{}, fmt.Errorf("marker: %w", err)
	}

	co, err := src.Sync(ctx, opts.URL, opts.Ref, opts.Auth)
	if err != nil {
		return Status{}, fmt.Errorf("fetch: %w", err)
	}

	tree, lay, err := layout.Resolve(co.Dir, opts.Host)
	if err != nil {
		return Status{}, fmt.Errorf("layout: %w", err)
	}

	drift, err := plan.Diff(tree, opts.Target, nil, nil)
	if err != nil {
		return Status{}, fmt.Errorf("diff: %w", err)
	}

	return Status{Applied: applied, Remote: co.Commit, Layout: lay, Drift: drift}, nil
}
