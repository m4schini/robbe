// SPDX-License-Identifier: TODO

// Package osexec implements adapters.Runner with os/exec.
package osexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Runner runs commands on the host.
type Runner struct{}

// New returns a host Runner.
func New() *Runner {
	return &Runner{}
}

// Run implements adapters.Runner.
func (*Runner) Run(ctx context.Context, name string, env []string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("run %s: %w", name, err)
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}
