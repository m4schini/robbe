// SPDX-License-Identifier: TODO

package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/m4schini/robbe/adapters/generator"
	"github.com/m4schini/robbe/adapters/gogit"
	"github.com/m4schini/robbe/adapters/osexec"
	"github.com/m4schini/robbe/adapters/systemctl"
	"github.com/m4schini/robbe/app/layout"
	"github.com/m4schini/robbe/app/sync"
	"github.com/m4schini/robbe/config"
	"github.com/m4schini/robbe/ports"
	"github.com/m4schini/robbe/telemetry"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	syncDryRun     bool
	syncAllowEmpty bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch the repository and converge the quadlet units of this host",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		log := telemetry.Logger("sync")

		runner := osexec.New()
		syncer := &sync.Syncer{
			Source:    gogit.New(cfg.Cache),
			Validator: generator.New(cfg.Generator, runner),
			Units:     systemctl.New(runner, cfg.User),
			Opts: sync.Options{
				URL:        cfg.Repo.URL,
				Ref:        cfg.Repo.Ref,
				Auth:       ports.Auth(cfg.Repo.Auth),
				Host:       cfg.Host,
				Target:     cfg.Target,
				Cache:      cfg.Cache,
				State:      cfg.State,
				Runtime:    resolveRuntime(cfg, log),
				User:       cfg.User,
				AllowEmpty: syncAllowEmpty,
			},
			Log: log,
			Now: nil,
		}

		res, err := syncer.Run(cmd.Context(), syncDryRun)

		out := cmd.OutOrStdout()
		printHeader(out, cfg, res)

		if err != nil {
			return fmt.Errorf("sync: %w", err)
		}

		switch {
		case res.UpToDate:
			fmt.Fprintln(out, "up to date")
		case syncDryRun:
			fmt.Fprintln(out)
			fmt.Fprint(out, res.Plan.String())
			fmt.Fprintln(out, "dry run: no changes made")
		default:
			fmt.Fprintln(out)
			fmt.Fprint(out, res.Plan.String())
			fmt.Fprintf(out, "applied %s\n", short(res.Commit))
		}

		return nil
	},
}

// resolveRuntime returns the directory for the lock file: cfg.Runtime when
// configured, otherwise the cache with a warning. The spec leaves the
// XDG_RUNTIME_DIR fallback to the application; the cache is per-user and
// not world-writable, unlike /tmp.
func resolveRuntime(cfg config.Config, log *zap.Logger) string {
	if cfg.Runtime != "" {
		return cfg.Runtime
	}

	log.Warn("XDG_RUNTIME_DIR unset, lock falls back to cache", zap.String("path", filepath.Join(cfg.Cache, "lock")))

	return cfg.Cache
}

// printHeader writes the repo/commit/host/target/state lines of the mockup.
func printHeader(out io.Writer, cfg config.Config, res sync.Result) {
	fmt.Fprintf(out, "repo    %s ref=%s\n", cfg.Repo.URL, cfg.Repo.Ref)

	applied := "none"
	if res.Previous != "" {
		applied = short(res.Previous)
	}

	fmt.Fprintf(out, "commit  %s (applied: %s)\n", short(res.Commit), applied)

	if res.Layout.Mode != "" {
		fmt.Fprintf(out, "host    %s  layout=%s\n", cfg.Host, describeLayout(res.Layout))
	}

	fmt.Fprintf(out, "target  %s\n", cfg.Target)
	fmt.Fprintf(out, "state   %s\n", cfg.State)
}

func describeLayout(lay layout.Layout) string {
	if lay.Mode == layout.ModeFlat {
		return "flat"
	}

	var parts []string
	if lay.CommonFound {
		parts = append(parts, "common")
	}

	if lay.HostDirFound {
		parts = append(parts, "host")
	}

	if len(parts) == 0 {
		return "hosts (nothing matched)"
	}

	return "hosts (" + strings.Join(parts, " + ") + ")"
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "print the plan and change nothing")
	syncCmd.Flags().BoolVar(&syncAllowEmpty, "allow-empty", false, "allow a run that removes every managed unit")
	rootCmd.AddCommand(syncCmd)
}
