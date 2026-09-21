// SPDX-License-Identifier: TODO

// Package gogit implements ports.Source with go-git.
package gogit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/m4schini/robbe/ports"
)

const (
	remoteName  = "origin"
	defaultUser = "git"
	// repoDirName is the clone location below the cache directory.
	repoDirName = "repo"
)

// ErrRefNotFound is returned when ref is neither a remote branch nor a tag.
var ErrRefNotFound = errors.New("ref not found on remote")

// ErrInsecureTransport is returned when a token would be sent over a
// transport that does not protect it, such as plain http.
var ErrInsecureTransport = errors.New("insecure transport")

// Source keeps a clone below cache and implements ports.Source.
type Source struct {
	cache string
}

// New returns a Source that stores its clone in <cache>/repo.
func New(cache string) *Source {
	return &Source{cache: cache}
}

// RepoDir is the directory the clone lives in.
func (s *Source) RepoDir() string {
	return filepath.Join(s.cache, repoDirName)
}

// Sync implements ports.Source.
func (s *Source) Sync(ctx context.Context, url, ref string, auth ports.Auth) (ports.Checkout, error) {
	repo, hash, err := s.fetch(ctx, url, ref, auth)
	if err != nil {
		return ports.Checkout{}, err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return ports.Checkout{}, fmt.Errorf("worktree: %w", err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return ports.Checkout{}, fmt.Errorf("checkout %s: %w", hash, err)
	}

	// Force checkout resets tracked files; Clean drops leftovers from
	// previous checkouts so the layout walk only sees committed files.
	if err := wt.Clean(&git.CleanOptions{Dir: true}); err != nil {
		return ports.Checkout{}, fmt.Errorf("clean worktree: %w", err)
	}

	return ports.Checkout{Dir: s.RepoDir(), Commit: hash.String()}, nil
}

// RemoteHead implements ports.Source.
func (s *Source) RemoteHead(ctx context.Context, url, ref string, auth ports.Auth) (string, error) {
	_, hash, err := s.fetch(ctx, url, ref, auth)
	if err != nil {
		return "", err
	}

	return hash.String(), nil
}

// fetch opens or clones the repository, fetches all branches and tags and
// resolves ref to a commit hash.
func (s *Source) fetch(ctx context.Context, url, ref string, auth ports.Auth) (*git.Repository, plumbing.Hash, error) {
	method, err := authMethod(url, auth)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}

	repo, err := s.open(ctx, url, method)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}

	err = repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: remoteName,
		RefSpecs: []gitconfig.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/tags/*:refs/tags/*",
		},
		Auth:  method,
		Force: true,
		Prune: true,
		Tags:  git.NoTags,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil, plumbing.ZeroHash, fmt.Errorf("fetch %s: %w", ports.RedactURL(url), err)
	}

	hash, err := resolve(repo, ref)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}

	return repo, hash, nil
}

// open returns the cached clone, cloning it first when it is missing or
// points at a different remote.
func (s *Source) open(ctx context.Context, url string, method transport.AuthMethod) (*git.Repository, error) {
	dir := s.RepoDir()

	repo, err := git.PlainOpen(dir)
	if err == nil {
		remote, rerr := repo.Remote(remoteName)
		if rerr == nil && len(remote.Config().URLs) == 1 && remote.Config().URLs[0] == url {
			return repo, nil
		}

		// Remote changed (or is unusable): start over.
		if err := os.RemoveAll(dir); err != nil {
			return nil, fmt.Errorf("remove stale clone: %w", err)
		}
	} else if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}

	if err := os.MkdirAll(s.cache, 0o755); err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	repo, err = git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:        url,
		RemoteName: remoteName,
		Auth:       method,
		NoCheckout: true,
		Tags:       git.NoTags,
	})
	if err != nil {
		return nil, fmt.Errorf("clone %s: %w", ports.RedactURL(url), err)
	}

	return repo, nil
}

// resolve maps ref to a commit: remote branch first, then tag (annotated
// tags are peeled), then a raw hash.
func resolve(repo *git.Repository, ref string) (plumbing.Hash, error) {
	candidates := []string{
		"refs/remotes/" + remoteName + "/" + ref,
		"refs/tags/" + ref,
		ref,
	}

	for _, c := range candidates {
		hash, err := repo.ResolveRevision(plumbing.Revision(c))
		if err == nil {
			return *hash, nil
		}
	}

	return plumbing.ZeroHash, fmt.Errorf("%w: %s", ErrRefNotFound, ref)
}

// authMethod picks the go-git auth for url in the order documented in
// docs/configuration.md: an ssh key file when configured, the ssh agent for
// ssh urls, basic auth when a token is set, else none. A token is never sent
// over plain http; that returns ErrInsecureTransport.
func authMethod(url string, auth ports.Auth) (transport.AuthMethod, error) {
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return nil, fmt.Errorf("parse url %q: %w", ports.RedactURL(url), err)
	}

	user := ep.User
	if user == "" {
		user = defaultUser
	}

	switch {
	case auth.SSHKey != "":
		keys, err := ssh.NewPublicKeysFromFile(user, auth.SSHKey, auth.SSHKeyPassword)
		if err != nil {
			return nil, fmt.Errorf("load ssh key %s: %w", auth.SSHKey, err)
		}

		return keys, nil
	case ep.Protocol == "ssh":
		agent, err := ssh.NewSSHAgentAuth(user)
		if err != nil {
			return nil, fmt.Errorf("ssh agent: %w", err)
		}

		return agent, nil
	case auth.Token != "":
		return tokenAuth(ep, auth)
	default:
		return nil, nil //nolint:nilnil // no auth needed for this url
	}
}

// tokenAuth builds basic auth from auth.Token, refusing plain http so the
// token is never sent in cleartext.
func tokenAuth(ep *transport.Endpoint, auth ports.Auth) (transport.AuthMethod, error) {
	if ep.Protocol == "http" {
		return nil, fmt.Errorf("%w: token auth over plain http", ErrInsecureTransport)
	}

	username := auth.Username
	if username == "" {
		username = defaultUser
	}

	return &http.BasicAuth{Username: username, Password: auth.Token}, nil
}
