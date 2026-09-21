// SPDX-License-Identifier: TODO

// Package marker reads and writes <state>/applied, the record of the last
// applied commit. It lives in the state directory, not in the target, so
// the target holds quadlet content only.
package marker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// File is the name of the marker file inside the state directory.
const File = "applied"

// filePerm is the mode of the marker file; it holds nothing secret.
const filePerm = 0o644

// ErrMalformed is returned when the marker file cannot be parsed.
var ErrMalformed = errors.New("malformed marker file")

// Marker records which commit was applied to the target and when.
type Marker struct {
	Commit string
	At     time.Time
}

// Read parses <stateDir>/applied. A missing file yields the zero Marker and
// no error.
func Read(stateDir string) (Marker, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, File))
	if errors.Is(err, os.ErrNotExist) {
		return Marker{}, nil
	}

	if err != nil {
		return Marker{}, fmt.Errorf("read marker: %w", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 2 { //nolint:mnd // two lines: commit, then timestamp
		return Marker{}, fmt.Errorf("%w: expected 2 lines, got %d", ErrMalformed, len(lines))
	}

	at, err := time.Parse(time.RFC3339, string(bytes.TrimSpace(lines[1])))
	if err != nil {
		return Marker{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	return Marker{Commit: string(bytes.TrimSpace(lines[0])), At: at}, nil
}

// Write writes the marker into stateDir, creating the directory if needed.
//
// The marker is written to a temp file in stateDir, fsynced, and renamed
// into place so that a crash mid-write never leaves a torn or empty marker
// behind; readers see either the old marker or the new one.
func Write(stateDir string, m Marker) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	content := m.Commit + "\n" + m.At.UTC().Format(time.RFC3339) + "\n"

	tmp, err := os.CreateTemp(stateDir, "."+File+"-*")
	if err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	tmpName := tmp.Name()

	if err := writeAndClose(tmp, content); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("write marker: %w", err)
	}

	if err := os.Rename(tmpName, filepath.Join(stateDir, File)); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("write marker: %w", err)
	}

	return nil
}

// writeAndClose writes content to f, makes it world-readable, flushes it to
// disk, and closes it. f is closed on every path.
func writeAndClose(f *os.File, content string) error {
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()

		return fmt.Errorf("write: %w", err)
	}

	// os.CreateTemp creates the file 0600; widen it before the rename.
	if err := f.Chmod(filePerm); err != nil {
		_ = f.Close()

		return fmt.Errorf("chmod: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()

		return fmt.Errorf("sync: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
