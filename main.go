// SPDX-License-Identifier: TODO

// Package main is the robbe entry point.
package main

import (
	"github.com/m4schini/robbe/cmd"
	"github.com/m4schini/robbe/config"
)

var version = "dev"

func main() {
	config.Version = version
	cmd.Execute()
}
