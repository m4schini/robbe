// SPDX-License-Identifier: TODO

// Package plan diffs the desired quadlet tree against the target directory
// and derives the systemd actions needed to converge.
package plan

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m4schini/robbe/app/layout"
	"github.com/m4schini/robbe/quadlet"
)

// HousekeepingPrefix marks robbe's own files in the target (.robbe-commit,
// .robbe-tmp-*); they are never part of the managed tree.
const HousekeepingPrefix = ".robbe"

// File is one entry of the plan. Unit is empty for support files (env
// files, configs, drop-ins).
type File struct {
	Rel  string
	Unit string
}

// Plan lists the file and unit changes needed to make target match desired.
//
// File lists are keyed by path; unit lists are keyed by unit name, so a
// unit whose source file moved or was renamed (same name, same content) is
// not restarted, and a unit whose ServiceName= changed is stopped under
// the old name and started under the new one.
type Plan struct {
	Add    []File
	Change []File
	Remove []File

	// Start holds units present in desired but not in target.
	Start []string
	// Restart holds units present in both whose source content differs,
	// plus every workload unit when RestartAll is set.
	Restart []string
	// Stop holds units present in target but not in desired.
	Stop []string
	// RestartAll is set when a support file was added, changed or removed.
	RestartAll bool
}

// Diff compares desired (relative path -> source file) with the files below
// targetDir. desiredUnits and targetUnits map relative paths to service
// names as reported by the generator; unit files missing from them fall
// back to quadlet.ConventionalUnit.
func Diff(desired layout.Tree, targetDir string, desiredUnits, targetUnits map[string]string) (Plan, error) {
	want, err := readDesired(desired)
	if err != nil {
		return Plan{}, err
	}

	have, err := readTarget(targetDir)
	if err != nil {
		return Plan{}, err
	}

	var p Plan

	for _, rel := range sortedKeys(want) {
		old, ok := have[rel]

		switch {
		case !ok:
			p.Add = append(p.Add, file(rel, desiredUnits))
		case !bytes.Equal(old, want[rel]):
			p.Change = append(p.Change, file(rel, desiredUnits))
		}
	}

	for _, rel := range sortedKeys(have) {
		if _, ok := want[rel]; !ok {
			p.Remove = append(p.Remove, file(rel, targetUnits))
		}
	}

	p.RestartAll = hasSupportFile(p.Add) || hasSupportFile(p.Change) || hasSupportFile(p.Remove)

	p.Start, p.Restart, p.Stop = diffUnits(byUnit(want, desiredUnits), byUnit(have, targetUnits), p.RestartAll)

	return p, nil
}

// unitSource is the file a unit is generated from.
type unitSource struct {
	rel     string
	content []byte
}

// byUnit indexes the unit files of a tree by unit name.
func byUnit(files map[string][]byte, units map[string]string) map[string]unitSource {
	out := map[string]unitSource{}

	for rel, content := range files {
		if !quadlet.IsUnitFile(rel) {
			continue
		}

		out[unitFor(rel, units)] = unitSource{rel: rel, content: content}
	}

	return out
}

// diffUnits derives the unit actions from the desired and current unit index.
func diffUnits(want, have map[string]unitSource, restartAll bool) (start, restart, stop []string) {
	for unit, src := range want {
		cur, ok := have[unit]

		switch {
		case !ok:
			start = append(start, unit)
		case !bytes.Equal(cur.content, src.content), restartAll && quadlet.IsWorkload(src.rel):
			restart = append(restart, unit)
		}
	}

	for unit := range have {
		if _, ok := want[unit]; !ok {
			stop = append(stop, unit)
		}
	}

	sort.Strings(start)
	sort.Strings(restart)
	sort.Strings(stop)

	return start, restart, stop
}

// Empty reports whether nothing needs to change.
func (p Plan) Empty() bool {
	return len(p.Add) == 0 && len(p.Change) == 0 && len(p.Remove) == 0 &&
		len(p.Start) == 0 && len(p.Restart) == 0 && len(p.Stop) == 0
}

// Units returns the number of unit actions, for logging.
func (p Plan) Units() int {
	return len(p.Start) + len(p.Restart) + len(p.Stop)
}

// String renders the plan for --dry-run output.
func (p Plan) String() string {
	var b strings.Builder

	b.WriteString("files\n")

	if len(p.Add)+len(p.Change)+len(p.Remove) == 0 {
		b.WriteString("  (none)\n")
	}

	writeFiles(&b, "+", p.Add)
	writeFiles(&b, "~", p.Change)
	writeFiles(&b, "-", p.Remove)

	b.WriteString("\nunits\n")

	if p.Units() == 0 {
		b.WriteString("  (none)\n")
	}

	for _, u := range p.Stop {
		fmt.Fprintf(&b, "  stop     %s\n", u)
	}

	for _, u := range p.Start {
		fmt.Fprintf(&b, "  start    %s\n", u)
	}

	for _, u := range p.Restart {
		fmt.Fprintf(&b, "  restart  %s\n", u)
	}

	return b.String()
}

func writeFiles(b *strings.Builder, sign string, files []File) {
	for _, f := range files {
		note := ""
		if f.Unit == "" {
			note = "          (support file -> restart all)"
		}

		fmt.Fprintf(b, "  %s %s%s\n", sign, f.Rel, note)
	}
}

func hasSupportFile(files []File) bool {
	for _, f := range files {
		if f.Unit == "" {
			return true
		}
	}

	return false
}

func file(rel string, units map[string]string) File {
	if !quadlet.IsUnitFile(rel) {
		return File{Rel: rel, Unit: ""}
	}

	return File{Rel: rel, Unit: unitFor(rel, units)}
}

func unitFor(rel string, units map[string]string) string {
	if unit, ok := units[rel]; ok {
		return unit
	}

	return quadlet.ConventionalUnit(rel)
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// readDesired loads the content of every desired file.
func readDesired(desired layout.Tree) (map[string][]byte, error) {
	files := make(map[string][]byte, len(desired))

	for rel, src := range desired {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}

		files[rel] = data
	}

	return files, nil
}

// readTarget loads every file below dir keyed by slash-separated relative
// path. robbe's housekeeping files are not part of the managed tree. A
// missing dir is treated as empty.
func readTarget(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}

			return err
		}

		if d.IsDir() || strings.HasPrefix(d.Name(), HousekeepingPrefix) {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}

		files[filepath.ToSlash(rel)] = nil

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read target %s: %w", dir, err)
	}

	// Read outside the walk callback; the target is robbe's own directory,
	// but this keeps the walk free of filesystem access.
	for rel := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}

		files[rel] = data
	}

	return files, nil
}
