// SPDX-License-Identifier: TODO

package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquire_CreatesDirAndExcludes(t *testing.T) {
	t.Parallel()

	runtime := filepath.Join(t.TempDir(), "run")

	unlock, err := Acquire(runtime)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(runtime, FileName)); err != nil {
		t.Errorf("lock file not created: %v", err)
	}

	if _, err := Acquire(runtime); !errors.Is(err, ErrLocked) {
		t.Errorf("second Acquire() error = %v, want ErrLocked", err)
	}

	unlock()

	unlock2, err := Acquire(runtime)
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}

	unlock2()
}
