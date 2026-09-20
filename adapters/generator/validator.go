// SPDX-License-Identifier: TODO

// Package generator implements ports.Validator with podman-system-generator.
package generator

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/m4schini/robbe/ports"
	"github.com/m4schini/robbe/quadlet"
)

// ErrValidation is returned when the generator rejects at least one file.
var ErrValidation = errors.New("quadlet validation failed")

var (
	unitHeader = regexp.MustCompile(`^---(.+)---$`)
	sourcePath = regexp.MustCompile(`^SourcePath=(.+)$`)
)

// Validator runs the quadlet generator through a ports.Runner.
type Validator struct {
	bin    string
	runner ports.Runner
}

// New returns a Validator using the generator at bin.
func New(bin string, runner ports.Runner) *Validator {
	return &Validator{bin: bin, runner: runner}
}

// Validate implements ports.Validator. On a failed run the units of the
// accepted files are still returned alongside the error. On a successful
// run every unit file in dir must have produced exactly one unit; a file
// without a unit or two files generating the same unit fail validation.
func (v *Validator) Validate(ctx context.Context, dir string, user bool) (ports.Report, error) {
	args := []string{"-dryrun"}
	if user {
		args = append([]string{"-user"}, args...)
	}

	stdout, stderr, runErr := v.runner.Run(ctx, v.bin, []string{"QUADLET_UNIT_DIRS=" + dir}, args...)

	units, dupes, err := parse(dir, stdout)
	if err != nil {
		return ports.Report{}, err
	}

	report := ports.Report{Units: units, Warnings: warnings(stderr)}

	if runErr != nil {
		return report, fmt.Errorf("%w: %s (%w)", ErrValidation, problems(stderr, stdout), runErr)
	}

	if len(dupes) > 0 {
		return report, fmt.Errorf("%w: several files generate the same unit: %s", ErrValidation, strings.Join(dupes, ", "))
	}

	missing, err := unitFilesWithoutUnit(dir, units)
	if err != nil {
		return report, err
	}

	if len(missing) > 0 {
		return report, fmt.Errorf("%w: no unit generated for %s", ErrValidation, strings.Join(missing, ", "))
	}

	return report, nil
}

// parse extracts the SourcePath -> unit mapping from dry-run output and
// reports unit names that appear more than once.
func parse(dir string, out []byte) (map[string]string, []string, error) {
	units := map[string]string{}
	seen := map[string]bool{}

	var (
		current string
		dupes   []string
	)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()

		if m := unitHeader.FindStringSubmatch(line); m != nil {
			current = m[1]

			if seen[current] {
				dupes = append(dupes, current)
			}

			seen[current] = true

			continue
		}

		m := sourcePath.FindStringSubmatch(line)
		if m == nil || current == "" {
			continue
		}

		rel, err := relative(dir, m[1])
		if err != nil {
			return nil, nil, err
		}

		units[rel] = current
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read generator output: %w", err)
	}

	sort.Strings(dupes)

	return units, dupes, nil
}

// unitFilesWithoutUnit lists quadlet files below dir that are absent from units.
func unitFilesWithoutUnit(dir string, units map[string]string) ([]string, error) {
	var missing []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}

		rel = filepath.ToSlash(rel)
		if _, ok := units[rel]; !ok && quadlet.IsUnitFile(rel) && !strings.HasPrefix(d.Name(), ".") {
			missing = append(missing, rel)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	sort.Strings(missing)

	return missing, nil
}

// relative maps an absolute SourcePath back to a slash path below dir. The
// generator prints the symlink-resolved path, so both spellings are tried.
func relative(dir, source string) (string, error) {
	rel, err := filepath.Rel(dir, source)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel), nil
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}

	rel, err = filepath.Rel(resolved, source)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: source path %s is outside %s", ErrValidation, source, dir)
	}

	return filepath.ToSlash(rel), nil
}

// problems collects the generator's error lines for the returned error.
func problems(stderr, stdout []byte) string {
	var lines []string

	for _, out := range [][]byte{stderr, stdout} {
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.Contains(line, "converting") {
				lines = append(lines, strings.TrimSpace(stripPrefix(line)))
			}
		}
	}

	if len(lines) == 0 {
		return strings.TrimSpace(string(stderr))
	}

	return strings.Join(lines, "; ")
}

// warnings collects generator stderr lines that are neither progress
// ("Loading source unit file") nor conversion errors.
func warnings(stderr []byte) []string {
	var lines []string

	for line := range strings.SplitSeq(string(stderr), "\n") {
		line = strings.TrimSpace(stripPrefix(line))
		if line == "" || strings.HasPrefix(line, "Loading source unit file") ||
			strings.Contains(line, "converting") || strings.HasPrefix(line, "processing encountered") {
			continue
		}

		lines = append(lines, line)
	}

	return lines
}

// stripPrefix drops the "quadlet-generator[PID]: " journal prefix.
func stripPrefix(line string) string {
	if _, rest, ok := strings.Cut(line, "]: "); ok && strings.HasPrefix(line, "quadlet-generator[") {
		return rest
	}

	return line
}
