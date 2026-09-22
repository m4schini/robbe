// SPDX-License-Identifier: TODO

// Package testutil holds helpers shared by the test suites of every package.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repo is a local git repository on branch main, served to go-git through
// the file transport (which needs git-upload-pack on PATH).
type Repo struct {
	Dir  string
	repo *git.Repository
	t    testing.TB
}

// NewRepo creates an empty repository in a temp dir. It skips the test when
// git-upload-pack is not installed.
func NewRepo(tb testing.TB) *Repo {
	tb.Helper()

	if _, err := exec.LookPath("git-upload-pack"); err != nil {
		tb.Skip("git-upload-pack not on PATH; go-git file transport needs it")
	}

	dir := tb.TempDir()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions:  git.InitOptions{DefaultBranch: plumbing.Main},
		Bare:         false,
		ObjectFormat: "",
	})
	if err != nil {
		tb.Fatalf("init repo: %v", err)
	}

	return &Repo{Dir: dir, repo: repo, t: tb}
}

// URL is the value to hand to adapters.Source for this repository.
func (r *Repo) URL() string {
	return r.Dir
}

// Commit writes files (relative path -> content), stages everything and
// commits. Returns the commit hash.
func (r *Repo) Commit(msg string, files map[string]string) string {
	r.t.Helper()

	wt, err := r.repo.Worktree()
	if err != nil {
		r.t.Fatalf("worktree: %v", err)
	}

	for rel, content := range files {
		path := filepath.Join(r.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.t.Fatalf("mkdir: %v", err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			r.t.Fatalf("write %s: %v", rel, err)
		}

		if _, err := wt.Add(rel); err != nil {
			r.t.Fatalf("add %s: %v", rel, err)
		}
	}

	hash, err := wt.Commit(msg, &git.CommitOptions{
		All:               false,
		AllowEmptyCommits: true,
		Author:            signature(),
		Committer:         signature(),
		Parents:           nil,
		SignKey:           nil,
		Signer:            nil,
		Amend:             false,
	})
	if err != nil {
		r.t.Fatalf("commit: %v", err)
	}

	return hash.String()
}

// Remove deletes files from the working tree and commits the removal.
func (r *Repo) Remove(msg string, rels ...string) string {
	r.t.Helper()

	wt, err := r.repo.Worktree()
	if err != nil {
		r.t.Fatalf("worktree: %v", err)
	}

	for _, rel := range rels {
		if _, err := wt.Remove(rel); err != nil {
			r.t.Fatalf("remove %s: %v", rel, err)
		}
	}

	hash, err := wt.Commit(msg, &git.CommitOptions{
		All:               false,
		AllowEmptyCommits: false,
		Author:            signature(),
		Committer:         signature(),
		Parents:           nil,
		SignKey:           nil,
		Signer:            nil,
		Amend:             false,
	})
	if err != nil {
		r.t.Fatalf("commit: %v", err)
	}

	return hash.String()
}

// Tag creates an annotated tag at HEAD.
func (r *Repo) Tag(name string) {
	r.t.Helper()

	head, err := r.repo.Head()
	if err != nil {
		r.t.Fatalf("head: %v", err)
	}

	_, err = r.repo.CreateTag(name, head.Hash(), &git.CreateTagOptions{
		Tagger:  signature(),
		Message: name,
		SignKey: nil,
	})
	if err != nil {
		r.t.Fatalf("tag %s: %v", name, err)
	}
}

// Head returns the current commit hash.
func (r *Repo) Head() string {
	r.t.Helper()

	head, err := r.repo.Head()
	if err != nil {
		r.t.Fatalf("head: %v", err)
	}

	return head.Hash().String()
}

func signature() *object.Signature {
	return &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}
}
