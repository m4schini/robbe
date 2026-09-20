// SPDX-License-Identifier: TODO

// Package cmd holds cli command implementation
package cmd

import (
	"os"

	"github.com/m4schini/robbe/config"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:          config.AppName,
	Short:        "Keep a host's Podman Quadlet units in sync with a git repository",
	SilenceUsage: true,
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
}
