// SPDX-License-Identifier: TODO

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/m4schini/robbe/adapters/osexec"
	"github.com/m4schini/robbe/app/install"
	"github.com/m4schini/robbe/telemetry"
	"github.com/spf13/cobra"
)

const defaultInterval = 5 * time.Minute

var installInterval time.Duration

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the robbe-sync service and timer and enable the timer",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		inst, err := newInstaller(installInterval)
		if err != nil {
			return err
		}

		if err := inst.Install(cmd.Context()); err != nil {
			return fmt.Errorf("install: %w", err)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "installed %s and %s in %s\n", install.ServiceUnit, install.TimerUnit, inst.Opts.UnitDir)
		fmt.Fprintf(out, "timer enabled, interval %s\n", installInterval)

		if inst.Opts.User {
			warnLinger(cmd, inst)
		}

		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Disable the robbe-sync timer and remove both units",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		inst, err := newInstaller(defaultInterval)
		if err != nil {
			return err
		}

		if err := inst.Uninstall(cmd.Context()); err != nil {
			return fmt.Errorf("uninstall: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "removed %s and %s from %s\n", install.ServiceUnit, install.TimerUnit, inst.Opts.UnitDir)

		return nil
	},
}

// newInstaller builds an Installer for the current user and executable.
func newInstaller(interval time.Duration) (*install.Installer, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home directory: %w", err)
	}

	user := os.Geteuid() != 0

	return &install.Installer{
		Runner: osexec.New(),
		Opts: install.Options{
			User:     user,
			UnitDir:  install.UnitDir(user, home),
			Binary:   binary,
			Interval: interval,
		},
		Log: telemetry.Logger("install"),
	}, nil
}

// warnLinger tells the user when the user instance stops with the session.
func warnLinger(cmd *cobra.Command, inst *install.Installer) {
	enabled, err := inst.LingerEnabled(cmd.Context(), os.Geteuid())
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check linger: %v\n", err)

		return
	}

	if !enabled {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: linger is not enabled; the timer only runs while you are logged in. Run: loginctl enable-linger %d\n", os.Geteuid())
	}
}

func init() {
	installCmd.Flags().DurationVar(&installInterval, "interval", defaultInterval, "time between sync runs")
	rootCmd.AddCommand(installCmd, uninstallCmd)
}
