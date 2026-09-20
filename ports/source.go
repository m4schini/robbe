// SPDX-License-Identifier: TODO

// Package ports defines the interfaces the application core depends on.
package ports

import "context"

// Auth holds the credentials used to reach the git remote. All fields are
// optional; see adapters/gogit for how they are combined.
type Auth struct {
	// SSHKey is the path to a private key file used for ssh remotes.
	SSHKey string
	// SSHKeyPassword unlocks SSHKey when the key is encrypted.
	SSHKeyPassword string
	// Token is used as the password for HTTP basic auth.
	Token string
	// Username is the HTTP basic auth user; defaults to "git".
	Username string
}

// Checkout describes a working tree checked out at a specific commit.
type Checkout struct {
	// Dir is the absolute path of the working tree.
	Dir string
	// Commit is the full hash the working tree is checked out at.
	Commit string
}

// Source fetches the configuration repository.
type Source interface {
	// Sync clones or updates the repository and checks out ref (a branch or
	// tag name). It returns the resulting working tree.
	Sync(ctx context.Context, url, ref string, auth Auth) (Checkout, error)
	// RemoteHead returns the commit hash ref currently points to on the
	// remote without touching the working tree.
	RemoteHead(ctx context.Context, url, ref string, auth Auth) (string, error)
}
