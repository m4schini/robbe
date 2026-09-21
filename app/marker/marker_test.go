// SPDX-License-Identifier: TODO

package marker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRead_Absent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	marker, err := Read(dir)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if marker.Commit != "" || !marker.At.IsZero() {
		t.Errorf("marker = %+v, want zero value", marker)
	}
}

func TestWriteRead_RoundTrip(t *testing.T) {
	t.Parallel()

	// The state directory does not exist yet; Write must create it.
	dir := filepath.Join(t.TempDir(), "state", "robbe")

	now := time.Now()
	want := Marker{Commit: "abc123", At: now}

	if err := Write(dir, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.Commit != want.Commit {
		t.Errorf("commit = %s, want %s", got.Commit, want.Commit)
	}

	wantAt := now.UTC().Truncate(time.Second)
	if !got.At.Equal(wantAt) {
		t.Errorf("at = %s, want %s", got.At, wantAt)
	}

	if got.At.Location() != time.UTC {
		t.Errorf("at location = %s, want UTC", got.At.Location())
	}

	if _, err := os.Stat(filepath.Join(dir, File)); err != nil {
		t.Errorf("stat %s: %v, want the marker inside the created state dir", File, err)
	}
}

func TestRead_Malformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "one line", content: "onlyonecommitline\n"},
		{name: "bad timestamp", content: "abc123\nnot-a-timestamp\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			path := filepath.Join(dir, File)
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write marker: %v", err)
			}

			_, err := Read(dir)
			if !errors.Is(err, ErrMalformed) {
				t.Errorf("err = %v, want ErrMalformed", err)
			}
		})
	}
}
