// SPDX-License-Identifier: TODO

package cmd

import (
	"fmt"
	"time"

	"github.com/m4schini/robbe/adapters/gogit"
	"github.com/m4schini/robbe/app/status"
	"github.com/m4schini/robbe/config"
	"github.com/m4schini/robbe/ports"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the applied commit, the remote head and drift",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		st, err := status.Get(cmd.Context(), gogit.New(cfg.Cache), status.Options{
			URL:    cfg.Repo.URL,
			Ref:    cfg.Repo.Ref,
			Auth:   ports.Auth(cfg.Repo.Auth),
			Host:   cfg.Host,
			Target: cfg.Target,
		})
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}

		out := cmd.OutOrStdout()
		if st.Applied.Commit == "" {
			fmt.Fprintln(out, "applied  none")
		} else {
			fmt.Fprintf(out, "applied  %s  %s\n", short(st.Applied.Commit), st.Applied.At.UTC().Format(time.RFC3339))
		}

		fmt.Fprintf(out, "remote   %s  %s\n", short(st.Remote), cfg.Repo.Ref)

		if st.Drift.Empty() {
			fmt.Fprintln(out, "drift    none")
		} else {
			fmt.Fprintf(out, "drift    %d to add, %d to change, %d to remove\n", len(st.Drift.Add), len(st.Drift.Change), len(st.Drift.Remove))
		}

		return nil
	},
}

// short abbreviates a commit hash for display.
func short(hash string) string {
	const n = 7
	if len(hash) > n {
		return hash[:n]
	}

	return hash
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
