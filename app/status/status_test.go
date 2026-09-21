// SPDX-License-Identifier: TODO

package status

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/m4schini/robbe/adapters/gogit"
	"github.com/m4schini/robbe/app/marker"
	"github.com/m4schini/robbe/internal/testutil"
	"github.com/m4schini/robbe/ports"
)

// noAuth is the zero-value ports.Auth; a bare ports.Auth{} composite literal
// trips exhaustruct_v5.
var noAuth ports.Auth

func TestGet(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Commit("initial", map[string]string{
		"nginx.container": "content",
	})

	target := t.TempDir()
	state := t.TempDir()
	src := gogit.New(t.TempDir())

	opts := Options{
		URL:    repo.URL(),
		Ref:    "main",
		Auth:   noAuth,
		Host:   "alpha",
		Target: target,
		State:  state,
	}

	st, err := Get(t.Context(), src, opts)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	var zero marker.Marker
	if st.Applied != zero {
		t.Errorf("Applied = %+v, want zero", st.Applied)
	}

	if st.Remote != repo.Head() {
		t.Errorf("Remote = %s, want %s", st.Remote, repo.Head())
	}

	addedNginx := false

	for _, f := range st.Drift.Add {
		if f.Rel == "nginx.container" {
			addedNginx = true
		}
	}

	if !addedNginx {
		t.Errorf("Drift.Add = %+v, want to contain nginx.container", st.Drift.Add)
	}

	if len(st.Drift.Start) != 1 || st.Drift.Start[0] != "nginx.service" {
		t.Errorf("Drift.Start = %v, want [nginx.service]", st.Drift.Start)
	}

	if err := marker.Write(state, marker.Marker{Commit: st.Remote, At: time.Now()}); err != nil {
		t.Fatalf("marker.Write() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(target, "nginx.container"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write nginx.container: %v", err)
	}

	st2, err := Get(t.Context(), src, opts)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !st2.Drift.Empty() {
		t.Errorf("Drift.Empty() = false, want true (drift = %+v)", st2.Drift)
	}

	if st2.Applied.Commit != st.Remote {
		t.Errorf("Applied.Commit = %s, want %s", st2.Applied.Commit, st.Remote)
	}
}
