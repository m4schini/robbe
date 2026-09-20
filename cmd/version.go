// SPDX-License-Identifier: TODO

package cmd

import (
	"fmt"

	"github.com/m4schini/robbe/config"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the robbe version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintln(cmd.OutOrStdout(), config.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
