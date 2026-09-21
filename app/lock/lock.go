// SPDX-License-Identifier: TODO

// Package lock serialises robbe runs that touch the shared clone: sync and
// status take the same exclusive flock so neither can reset the worktree
// while the other reads it.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	// FileName is the lock file's name below the runtime directory.
	FileName = "lock"
	filePerm = 0o644
	dirPerm  = 0o755
)

// ErrLocked reports that another robbe run holds the lock.
var ErrLocked = errors.New("another robbe run is in progress")

// Acquire takes an exclusive flock on <runtime>/lock so a timer run, a
// manual run and a status call cannot interleave. The runtime directory is
// created when missing (systemd creates it for timer runs, manual runs do
// it here). The returned func releases the lock.
func Acquire(runtime string) (func(), error) {
	if err := os.MkdirAll(runtime, dirPerm); err != nil {
		return nil, fmt.Errorf("create runtime dir: %w", err)
	}

	f, err := os.OpenFile(filepath.Join(runtime, FileName), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()

		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}

		return nil, fmt.Errorf("lock: %w", err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
