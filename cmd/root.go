// SPDX-License-Identifier: TODO

// Package cmd holds cli command implementation
package cmd

import (
	"fmt"
	"os"

	"github.com/m4schini/robbe/config"
	"github.com/m4schini/robbe/internal/ansi"
	"github.com/spf13/cobra"
)

// colorMode is the value of the persistent --color flag.
var colorMode string

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:          config.AppName,
	Short:        "Keep a host's Podman Quadlet units in sync with a git repository",
	SilenceUsage: true,
	// Validate --color before any subcommand loads config or touches the
	// repository, so a typo fails fast with a single line.
	PersistentPreRunE: func(*cobra.Command, []string) error {
		if err := ansi.ValidMode(colorMode); err != nil {
			return fmt.Errorf("--color: %w", err)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(config.Init)
	rootCmd.PersistentFlags().StringVar(&config.ConfigFile, "config", "", "config file (default: XDG search, see docs/configuration.md)")
	rootCmd.PersistentFlags().StringVar(&colorMode, "color", ansi.ModeAuto, "colorize output: auto, always or never")
}
