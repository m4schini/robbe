// SPDX-License-Identifier: TODO

// Package layout resolves the desired quadlet tree for a host from a
// repository checkout.
package layout

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	commonDir = "common"
	hostsDir  = "hosts"
	// kubeExt marks quadlet files robbe refuses to manage.
	kubeExt = ".kube"
)

// Mode says which repository layout was detected.
type Mode string

const (
	// ModeFlat means the repository root is the desired tree.
	ModeFlat Mode = "flat"
	// ModeHosts means common/ overlaid by hosts/<host>/ is the desired tree.
	ModeHosts Mode = "hosts"
)

// Tree maps a path relative to the target directory to the absolute source
// file in the checkout.
type Tree map[string]string

// Layout describes how the tree was assembled.
type Layout struct {
	Mode Mode
	// HostDirFound is false when hosts/ exists but hosts/<host>/ does not.
	HostDirFound bool
	// CommonFound reports whether common/ exists (hosts mode only).
	CommonFound bool
	// Skipped lists relative paths of .kube files that were ignored.
	Skipped []string
}

// Resolve builds the desired tree for host from repoDir. In hosts mode the
// host directory overrides common/ on equal relative paths.
func Resolve(repoDir, host string) (Tree, Layout, error) {
	hostsPath := filepath.Join(repoDir, hostsDir)

	hostsExists, err := isDir(hostsPath)
	if err != nil {
		return nil, Layout{}, err
	}

	tree := Tree{}

	if !hostsExists {
		layout := Layout{Mode: ModeFlat, HostDirFound: true, CommonFound: false, Skipped: nil}

		skipped, err := walkInto(tree, repoDir)
		if err != nil {
			return nil, Layout{}, err
		}

		layout.Skipped = skipped

		return tree, layout, nil
	}

	layout := Layout{Mode: ModeHosts, HostDirFound: false, CommonFound: false, Skipped: nil}

	for _, dir := range []struct {
		path  string
		found *bool
	}{
		{filepath.Join(repoDir, commonDir), &layout.CommonFound},
		{filepath.Join(hostsPath, host), &layout.HostDirFound},
	} {
		exists, err := isDir(dir.path)
		if err != nil {
			return nil, Layout{}, err
		}

		*dir.found = exists
		if !exists {
			continue
		}

		skipped, err := walkInto(tree, dir.path)
		if err != nil {
			return nil, Layout{}, err
		}

		layout.Skipped = append(layout.Skipped, skipped...)
	}

	sort.Strings(layout.Skipped)

	return tree, layout, nil
}

// walkInto adds every regular file below root to tree (later calls override
// earlier ones). Git and robbe housekeeping entries (.git*, .robbe*) are
// skipped, .kube files are reported.
func walkInto(tree Tree, root string) ([]string, error) {
	var skipped []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == root {
			return nil
		}

		if isHousekeeping(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}

		rel = filepath.ToSlash(rel)

		if strings.HasSuffix(rel, kubeExt) {
			skipped = append(skipped, rel)

			return nil
		}

		tree[rel] = path

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	return skipped, nil
}

// Paths returns the relative paths of the tree in sorted order.
func (t Tree) Paths() []string {
	paths := make([]string, 0, len(t))
	for rel := range t {
		paths = append(paths, rel)
	}

	sort.Strings(paths)

	return paths
}

// isHousekeeping reports names that never belong to the desired tree:
// git metadata (.git, .gitignore, ...) and robbe's own files (.robbe-commit,
// .robbe-tmp-*).
func isHousekeeping(name string) bool {
	return strings.HasPrefix(name, ".git") || strings.HasPrefix(name, ".robbe")
}

func isDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}

	return info.IsDir(), nil
}
