// SPDX-License-Identifier: TODO

package gogit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/m4schini/robbe/internal/testutil"
	"github.com/m4schini/robbe/ports"
)

// noAuth is the zero-value ports.Auth used wherever no credentials are
// needed; a bare ports.Auth{} composite literal trips exhaustruct_v5.
var noAuth ports.Auth

// assertFileContent fails the test unless the file at path exists and holds want.
func assertFileContent(tb testing.TB, path, want string) {
	tb.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}

	if string(got) != want {
		tb.Errorf("%s = %q, want %q", path, got, want)
	}
}

// assertFileAbsent fails the test if path exists.
func assertFileAbsent(tb testing.TB, path string) {
	tb.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		tb.Errorf("%s = exists (err=%v), want absent", path, err)
	}
}

func TestSync_Branch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Commit("initial", map[string]string{
		"a.container": "a-content",
		"sub/b.env":   "b-content",
	})

	src := New(t.TempDir())

	checkout, err := src.Sync(t.Context(), repo.URL(), "main", noAuth)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	if checkout.Commit != repo.Head() {
		t.Errorf("commit = %s, want %s", checkout.Commit, repo.Head())
	}

	if checkout.Dir != src.RepoDir() {
		t.Errorf("dir = %s, want %s", checkout.Dir, src.RepoDir())
	}

	assertFileContent(t, filepath.Join(checkout.Dir, "a.container"), "a-content")
	assertFileContent(t, filepath.Join(checkout.Dir, "sub/b.env"), "b-content")
}

func TestSync_Tag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	c1 := repo.Commit("c1", map[string]string{"c1.txt": "one"})
	repo.Tag("v1")
	repo.Commit("c2", map[string]string{"c2.txt": "two"})

	src := New(t.TempDir())

	checkout, err := src.Sync(t.Context(), repo.URL(), "v1", noAuth)
	if err != nil {
		t.Fatalf("sync v1: %v", err)
	}

	if checkout.Commit != c1 {
		t.Errorf("commit = %s, want %s", checkout.Commit, c1)
	}

	assertFileContent(t, filepath.Join(checkout.Dir, "c1.txt"), "one")
	assertFileAbsent(t, filepath.Join(checkout.Dir, "c2.txt"))

	checkout, err = src.Sync(t.Context(), repo.URL(), "main", noAuth)
	if err != nil {
		t.Fatalf("sync main: %v", err)
	}

	if checkout.Commit != repo.Head() {
		t.Errorf("commit = %s, want %s", checkout.Commit, repo.Head())
	}

	assertFileContent(t, filepath.Join(checkout.Dir, "c2.txt"), "two")
}

func TestSync_FetchNewCommit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Commit("c1", map[string]string{"old.txt": "old"})

	src := New(t.TempDir())

	if _, err := src.Sync(t.Context(), repo.URL(), "main", noAuth); err != nil {
		t.Fatalf("sync 1: %v", err)
	}

	repo.Commit("c2", map[string]string{"new.txt": "new"})
	repo.Remove("c3", "old.txt")

	checkout, err := src.Sync(t.Context(), repo.URL(), "main", noAuth)
	if err != nil {
		t.Fatalf("sync 2: %v", err)
	}

	if checkout.Commit != repo.Head() {
		t.Errorf("commit = %s, want %s", checkout.Commit, repo.Head())
	}

	assertFileContent(t, filepath.Join(checkout.Dir, "new.txt"), "new")
	assertFileAbsent(t, filepath.Join(checkout.Dir, "old.txt"))
}

func TestSync_RefNotFound(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Commit("c1", map[string]string{"a.txt": "a"})

	src := New(t.TempDir())

	_, err := src.Sync(t.Context(), repo.URL(), "nope", noAuth)
	if !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("err = %v, want ErrRefNotFound", err)
	}
}

func TestRemoteHead(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Commit("c1", map[string]string{"a.txt": "a"})

	src := New(t.TempDir())

	hash, err := src.RemoteHead(t.Context(), repo.URL(), "main", noAuth)
	if err != nil {
		t.Fatalf("remote head: %v", err)
	}

	if hash != repo.Head() {
		t.Errorf("hash = %s, want %s", hash, repo.Head())
	}

	entries, err := os.ReadDir(src.RepoDir())
	if err != nil {
		t.Fatalf("read repo dir: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != ".git" {
		t.Errorf("repo dir entries = %v, want only .git", entries)
	}
}

func TestSync_RemoteChanged(t *testing.T) {
	t.Parallel()

	repoA := testutil.NewRepo(t)
	repoA.Commit("a", map[string]string{"a.txt": "a"})

	repoB := testutil.NewRepo(t)
	repoB.Commit("b", map[string]string{"b.txt": "b"})

	src := New(t.TempDir())

	if _, err := src.Sync(t.Context(), repoA.URL(), "main", noAuth); err != nil {
		t.Fatalf("sync A: %v", err)
	}

	checkout, err := src.Sync(t.Context(), repoB.URL(), "main", noAuth)
	if err != nil {
		t.Fatalf("sync B: %v", err)
	}

	if checkout.Commit != repoB.Head() {
		t.Errorf("commit = %s, want %s", checkout.Commit, repoB.Head())
	}

	assertFileContent(t, filepath.Join(checkout.Dir, "b.txt"), "b")
	assertFileAbsent(t, filepath.Join(checkout.Dir, "a.txt"))
}

func TestAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		auth ports.Auth
		want func(tb testing.TB, method transport.AuthMethod, err error)
	}{
		{
			name: "https token",
			url:  "https://github.com/me/x.git",
			auth: ports.Auth{SSHKey: "", SSHKeyPassword: "", Token: "tok", Username: ""},
			want: func(tb testing.TB, method transport.AuthMethod, err error) {
				tb.Helper()

				if err != nil {
					tb.Fatalf("err = %v, want nil", err)
				}

				basic, ok := method.(*http.BasicAuth)
				if !ok {
					tb.Fatalf("method = %T, want *http.BasicAuth", method)
				}

				if basic.Username != "git" || basic.Password != "tok" {
					tb.Errorf("basic = %+v, want Username=git Password=tok", basic)
				}
			},
		},
		{
			name: "https token with username",
			url:  "https://github.com/me/x.git",
			auth: ports.Auth{SSHKey: "", SSHKeyPassword: "", Token: "tok", Username: "me"},
			want: func(tb testing.TB, method transport.AuthMethod, err error) {
				tb.Helper()

				if err != nil {
					tb.Fatalf("err = %v, want nil", err)
				}

				basic, ok := method.(*http.BasicAuth)
				if !ok {
					tb.Fatalf("method = %T, want *http.BasicAuth", method)
				}

				if basic.Username != "me" || basic.Password != "tok" {
					tb.Errorf("basic = %+v, want Username=me Password=tok", basic)
				}
			},
		},
		{
			name: "https no auth",
			url:  "https://github.com/me/x.git",
			auth: noAuth,
			want: func(tb testing.TB, method transport.AuthMethod, err error) {
				tb.Helper()

				if err != nil {
					tb.Errorf("err = %v, want nil", err)
				}

				if method != nil {
					tb.Errorf("method = %v, want nil", method)
				}
			},
		},
		{
			name: "nonexistent ssh key",
			url:  "https://github.com/me/x.git",
			auth: ports.Auth{SSHKey: "/nonexistent", SSHKeyPassword: "", Token: "", Username: ""},
			want: func(tb testing.TB, method transport.AuthMethod, err error) {
				tb.Helper()

				if err == nil {
					tb.Fatalf("err = nil, want error")
				}

				if method != nil {
					tb.Errorf("method = %v, want nil", method)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			method, err := authMethod(tt.url, tt.auth)
			tt.want(t, method, err)
		})
	}

	t.Run("ssh agent", func(t *testing.T) {
		t.Parallel()

		if os.Getenv("SSH_AUTH_SOCK") == "" {
			t.Skip("no SSH_AUTH_SOCK in environment")
		}

		method, err := authMethod("git@github.com:me/x.git", noAuth)
		if err != nil {
			// Connecting to whatever SSH_AUTH_SOCK points at can fail in ways
			// unrelated to authMethod itself; only check the happy path.
			return
		}

		if _, ok := method.(*ssh.PublicKeysCallback); !ok {
			t.Errorf("method = %T, want *ssh.PublicKeysCallback", method)
		}
	})
}
