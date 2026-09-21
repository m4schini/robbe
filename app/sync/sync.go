// SPDX-License-Identifier: TODO

// Package sync implements one robbe sync run: fetch, resolve, validate, plan
// and apply.
package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m4schini/robbe/app/layout"
	"github.com/m4schini/robbe/app/lock"
	"github.com/m4schini/robbe/app/marker"
	"github.com/m4schini/robbe/app/plan"
	"github.com/m4schini/robbe/ports"
	"github.com/m4schini/robbe/quadlet"
	"go.uber.org/zap"
)

const (
	stagingDirName = "staging"
	tmpPrefix      = ".robbe-tmp-"
	filePerm       = 0o644
	dirPerm        = 0o755
)

var (
	// ErrLocked is returned when another robbe run holds the cache lock.
	ErrLocked = lock.ErrLocked
	// ErrEmptyTree is returned when the desired tree is empty while the
	// target still holds managed files and AllowEmpty is not set.
	ErrEmptyTree = errors.New("desired tree is empty; refusing to remove every managed unit (use --allow-empty)")
	// ErrUnitsFailed is returned when systemd could not bring a unit into
	// the requested state.
	ErrUnitsFailed = errors.New("unit actions failed")
)

// isUp reports whether a systemd ActiveState counts as running.
func isUp(state string) bool {
	switch state {
	case "active", "activating", "reloading":
		return true
	default:
		return false
	}
}

// Options are the parts of the configuration a run needs.
type Options struct {
	URL    string
	Ref    string
	Auth   ports.Auth
	Host   string
	Target string
	Cache  string
	// State is the directory holding the applied-commit marker.
	State string
	// Runtime is the directory holding the lock file. The caller resolves
	// it (XDG_RUNTIME_DIR, /run, or the cache fallback); it is never empty.
	Runtime string
	User    bool
	// AllowEmpty permits a run that removes every managed file.
	AllowEmpty bool
}

// Syncer wires the ports together. Zero-value Log and Now are replaced by a
// no-op logger and time.Now.
type Syncer struct {
	Source    ports.Source
	Validator ports.Validator
	Units     ports.UnitManager
	Opts      Options
	Log       *zap.Logger
	Now       func() time.Time
}

// Result describes what a run found and did.
type Result struct {
	// Commit is the fetched head of ref.
	Commit string
	// Previous is the commit recorded in the marker before the run ("" if none).
	Previous string
	Layout   layout.Layout
	Plan     plan.Plan
	// UpToDate is set when the marker already named Commit and nothing was done.
	UpToDate bool
	// Applied is set when files were written and units were driven.
	Applied bool
}

// Run performs a sync. With dryRun it stops after computing the plan.
func (s *Syncer) Run(ctx context.Context, dryRun bool) (Result, error) {
	unlock, err := lock.Acquire(s.Opts.Runtime)
	if err != nil {
		return Result{}, fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	log := s.logger()

	co, err := s.Source.Sync(ctx, s.Opts.URL, s.Opts.Ref, s.Opts.Auth)
	if err != nil {
		return Result{}, fmt.Errorf("fetch: %w", err)
	}

	log.Info("fetched", zap.String("url", ports.RedactURL(s.Opts.URL)), zap.String("ref", s.Opts.Ref), zap.String("commit", co.Commit))

	applied, err := marker.Read(s.Opts.State)
	if err != nil {
		return Result{}, fmt.Errorf("marker: %w", err)
	}

	var res Result
	res.Commit = co.Commit
	res.Previous = applied.Commit

	upToDate, err := s.upToDate(applied.Commit, co.Commit)
	if err != nil {
		return Result{}, err
	}

	if upToDate {
		res.UpToDate = true

		return res, nil
	}

	staging := filepath.Join(s.Opts.Cache, stagingDirName)

	if err := s.computePlan(ctx, co.Dir, staging, &res); err != nil {
		return Result{}, err
	}

	if dryRun {
		return res, nil
	}

	if err := s.apply(ctx, staging, res.Plan); err != nil {
		return res, err
	}

	res.Applied = true

	if err := marker.Write(s.Opts.State, marker.Marker{Commit: co.Commit, At: s.now()}); err != nil {
		return res, fmt.Errorf("marker: %w", err)
	}

	log.Info("applied", zap.String("commit", co.Commit))

	return res, nil
}

// upToDate reports whether the run can stop: the marker already names the
// fetched commit and the target directory still exists. A marker for the
// fetched commit with a missing target (wiped by hand, fresh host with a
// copied state dir) is re-applied.
func (s *Syncer) upToDate(applied, fetched string) (bool, error) {
	if applied != fetched {
		return false, nil
	}

	targetExists, err := dirExists(s.Opts.Target)
	if err != nil {
		return false, fmt.Errorf("target: %w", err)
	}

	if !targetExists {
		s.logger().Info("target missing, re-applying", zap.String("commit", fetched), zap.String("target", s.Opts.Target))

		return false, nil
	}

	s.logger().Info("no changes", zap.String("commit", fetched))

	return true, nil
}

// computePlan performs steps 3-5: layout, stage + validate, diff. It fills
// res.Layout and res.Plan.
func (s *Syncer) computePlan(ctx context.Context, repoDir, staging string, res *Result) error {
	log := s.logger()

	tree, lay, err := layout.Resolve(repoDir, s.Opts.Host)
	if err != nil {
		return fmt.Errorf("layout: %w", err)
	}

	res.Layout = lay
	s.logLayout(lay)

	desiredUnits, err := s.stageAndValidate(ctx, staging, tree)
	if err != nil {
		return err
	}

	log.Info("validated", zap.Int("units", len(desiredUnits)), zap.String("dir", staging))

	if err := removeStaleTemp(s.Opts.Target); err != nil {
		return err
	}

	p, err := plan.Diff(tree, s.Opts.Target, desiredUnits, s.targetUnits(ctx))
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	if len(tree) == 0 && len(p.Remove) > 0 && !s.Opts.AllowEmpty {
		return ErrEmptyTree
	}

	if err := s.addInactive(ctx, tree, desiredUnits, &p); err != nil {
		return err
	}

	res.Plan = p
	log.Info("plan",
		zap.Int("add", len(p.Add)), zap.Int("change", len(p.Change)), zap.Int("remove", len(p.Remove)),
		zap.Int("start", len(p.Start)), zap.Int("restart", len(p.Restart)), zap.Int("stop", len(p.Stop)),
		zap.Bool("restart_all", p.RestartAll))

	return nil
}

// addInactive adds every desired workload unit that is not active to
// p.Start, so a unit whose start failed in an earlier run (or that was
// stopped by hand) is retried on the next non-short-circuited run.
func (s *Syncer) addInactive(ctx context.Context, tree layout.Tree, units map[string]string, p *plan.Plan) error {
	candidates := untouchedWorkloads(tree, units, p)
	if len(candidates) == 0 {
		return nil
	}

	states, err := s.Units.ActiveStates(ctx, candidates...)
	if err != nil {
		return fmt.Errorf("unit states: %w", err)
	}

	for _, unit := range candidates {
		if !isUp(states[unit]) {
			s.logger().Warn("unit not active, scheduling start", zap.String("unit", unit), zap.String("state", states[unit]))
			p.Start = append(p.Start, unit)
		}
	}

	sort.Strings(p.Start)

	return nil
}

// untouchedWorkloads lists the desired .container/.pod units the plan does
// not already start or restart.
func untouchedWorkloads(tree layout.Tree, units map[string]string, p *plan.Plan) []string {
	touched := map[string]bool{}
	for _, u := range append(append([]string(nil), p.Start...), p.Restart...) {
		touched[u] = true
	}

	var out []string

	for _, rel := range tree.Paths() {
		if !quadlet.IsWorkload(rel) {
			continue
		}

		unit, ok := units[rel]
		if !ok {
			unit = quadlet.ConventionalUnit(rel)
		}

		if !touched[unit] {
			out = append(out, unit)
		}
	}

	return out
}

// stageAndValidate copies the desired tree into staging and runs the
// generator over it. staging is recreated on every run.
func (s *Syncer) stageAndValidate(ctx context.Context, staging string, tree layout.Tree) (map[string]string, error) {
	if err := os.RemoveAll(staging); err != nil {
		return nil, fmt.Errorf("clear staging: %w", err)
	}

	if err := os.MkdirAll(staging, dirPerm); err != nil {
		return nil, fmt.Errorf("create staging: %w", err)
	}

	root, err := os.OpenRoot(staging)
	if err != nil {
		return nil, fmt.Errorf("open staging: %w", err)
	}
	defer root.Close()

	for rel, src := range tree {
		if err := copyFile(root, src, filepath.FromSlash(rel)); err != nil {
			return nil, fmt.Errorf("stage %s: %w", rel, err)
		}
	}

	report, err := s.Validator.Validate(ctx, staging, s.Opts.User)
	s.logWarnings(report.Warnings)

	if err != nil {
		s.logger().Error("validation failed", zap.Error(err))

		return nil, fmt.Errorf("validate: %w", err)
	}

	return report.Units, nil
}

// targetUnits asks the generator for the unit names of the files currently
// in the target, so removed units are stopped by their real name. Files the
// generator rejects fall back to the naming convention in the planner.
func (s *Syncer) targetUnits(ctx context.Context) map[string]string {
	if _, err := os.Stat(s.Opts.Target); err != nil {
		return nil
	}

	report, err := s.Validator.Validate(ctx, s.Opts.Target, s.Opts.User)
	if err != nil {
		s.logger().Warn("target has invalid quadlets, using conventional unit names for them", zap.Error(err))
	}

	return report.Units
}

// apply performs steps 6-9: stop and remove, write, daemon-reload, then one
// restart transaction for started and restarted units. Unit failures are
// collected; the caller leaves the marker unchanged when apply returns an
// error.
func (s *Syncer) apply(ctx context.Context, staging string, p plan.Plan) error {
	var errs []error

	stopped, err := s.stop(ctx, p.Stop)
	if err != nil {
		errs = append(errs, err)
	}

	if err := s.writeFiles(staging, p, stopped); err != nil {
		return errors.Join(append(errs, err)...)
	}

	if err := s.Units.DaemonReload(ctx); err != nil {
		return errors.Join(append(errs, err)...)
	}

	if err := s.restart(ctx, p.Start, p.Restart); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// stop stops units and reports which of them are down afterwards. A unit
// systemd no longer knows counts as stopped.
func (s *Syncer) stop(ctx context.Context, units []string) (map[string]bool, error) {
	stopped := map[string]bool{}
	if len(units) == 0 {
		return stopped, nil
	}

	log := s.logger()
	s.logUnits("stop", units)

	stopErr := s.Units.Stop(ctx, units...)

	states, err := s.Units.ActiveStates(ctx, units...)
	if err != nil {
		return stopped, errors.Join(stopErr, fmt.Errorf("unit states: %w", err))
	}

	var failed []string

	for _, unit := range units {
		if isUp(states[unit]) || states[unit] == "deactivating" {
			failed = append(failed, unit)
			log.Error("unit still running", zap.String("unit", unit), zap.String("state", states[unit]))

			continue
		}

		stopped[unit] = true
	}

	if len(failed) > 0 {
		return stopped, fmt.Errorf("%w: stop %s: %w", ErrUnitsFailed, strings.Join(failed, ", "), stopErr)
	}

	if stopErr != nil {
		log.Warn("stop reported an error but every unit is down", zap.Error(stopErr))
	}

	return stopped, nil
}

// restart brings started and restarted units up in one transaction so
// systemd orders them by their dependencies, then verifies each is active.
func (s *Syncer) restart(ctx context.Context, start, restart []string) error {
	units := append(append([]string(nil), start...), restart...)
	if len(units) == 0 {
		return nil
	}

	log := s.logger()
	s.logUnits("start", start)
	s.logUnits("restart", restart)

	restartErr := s.Units.Restart(ctx, units...)

	states, err := s.Units.ActiveStates(ctx, units...)
	if err != nil {
		return errors.Join(restartErr, fmt.Errorf("unit states: %w", err))
	}

	var failed []string

	for _, unit := range units {
		if !isUp(states[unit]) {
			failed = append(failed, unit)
			log.Error("unit not active", zap.String("unit", unit), zap.String("state", states[unit]))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%w: %s: %w", ErrUnitsFailed, strings.Join(failed, ", "), restartErr)
	}

	if restartErr != nil {
		log.Warn("restart reported an error but every unit is active", zap.Error(restartErr))
	}

	return nil
}

func (s *Syncer) logUnits(action string, units []string) {
	if len(units) > 0 {
		s.logger().Info("units", zap.String("action", action), zap.Strings("units", units))
	}
}

// writeFiles removes the files of p.Remove and installs p.Add and p.Change
// from staging. A unit file whose unit the plan stops is only removed when
// the stop succeeded, so a failed stop is retried on the next run. Files of
// units the plan does not stop (a unit file moved to a new path, whose unit
// stays up) are removed unconditionally.
func (s *Syncer) writeFiles(staging string, p plan.Plan, stopped map[string]bool) error {
	log := s.logger()

	mustBeDown := make(map[string]bool, len(p.Stop))
	for _, u := range p.Stop {
		mustBeDown[u] = true
	}

	for _, f := range p.Remove {
		if f.Unit != "" && mustBeDown[f.Unit] && !stopped[f.Unit] {
			continue
		}

		if err := removeFile(s.Opts.Target, f.Rel); err != nil {
			return err
		}

		log.Info("file", zap.String("action", "remove"), zap.String("path", f.Rel))
	}

	for _, f := range append(append([]plan.File(nil), p.Add...), p.Change...) {
		if err := installFile(staging, s.Opts.Target, f.Rel); err != nil {
			return err
		}

		log.Info("file", zap.String("action", "write"), zap.String("path", f.Rel))
	}

	return nil
}

func (s *Syncer) logLayout(lay layout.Layout) {
	log := s.logger()

	log.Info("layout", zap.String("mode", string(lay.Mode)), zap.String("host", s.Opts.Host), zap.Bool("common", lay.CommonFound))

	if lay.Mode == layout.ModeHosts && !lay.HostDirFound {
		log.Warn("host dir missing", zap.String("host", s.Opts.Host), zap.String("applying", "common"))
	}

	for _, rel := range lay.Skipped {
		log.Warn("unsupported file skipped", zap.String("path", rel), zap.String("reason", ".kube units are not managed"))
	}
}

func (s *Syncer) logWarnings(warnings []string) {
	for _, w := range warnings {
		s.logger().Warn("generator", zap.String("message", w))
	}
}

func (s *Syncer) logger() *zap.Logger {
	if s.Log == nil {
		return zap.NewNop()
	}

	return s.Log
}

func (s *Syncer) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}

	return s.Now()
}

// installFile copies staging/rel into target/rel atomically (temp file in
// the same directory, then rename).
func installFile(staging, target, rel string) error {
	src := filepath.Join(staging, filepath.FromSlash(rel))
	dst := filepath.Join(target, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), tmpPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	tmpName := tmp.Name()

	if err := writeAndClose(tmp, src); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("write %s: %w", rel, err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("rename %s: %w", rel, err)
	}

	return nil
}

func writeAndClose(dst *os.File, src string) error {
	in, err := os.Open(src)
	if err != nil {
		_ = dst.Close()

		return fmt.Errorf("open: %w", err)
	}
	defer in.Close()

	if _, err := io.Copy(dst, in); err != nil {
		_ = dst.Close()

		return fmt.Errorf("copy: %w", err)
	}

	if err := dst.Chmod(filePerm); err != nil {
		_ = dst.Close()

		return fmt.Errorf("chmod: %w", err)
	}

	if err := dst.Sync(); err != nil {
		_ = dst.Close()

		return fmt.Errorf("sync: %w", err)
	}

	if err := dst.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

// removeFile deletes target/rel and any parent directories left empty
// below target. The target directory itself is kept.
func removeFile(target, rel string) error {
	// Clean target so the prune guard's string comparison matches the
	// cleaned paths produced by filepath.Dir; an unclean target (trailing
	// slash, "./x") would otherwise let the walk remove target and its
	// ancestors.
	target = filepath.Clean(target)
	path := filepath.Join(target, filepath.FromSlash(rel))

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", rel, err)
	}

	pruneEmptyDirs(target, filepath.Dir(path))

	return nil
}

// pruneEmptyDirs removes dir and its parents up to (excluding) target while
// they are empty. Errors (directory not empty, already gone) end the walk.
func pruneEmptyDirs(target, dir string) {
	for dir != target && dir != "." {
		if os.Remove(dir) != nil {
			return
		}

		dir = filepath.Dir(dir)
	}
}

// dirExists reports whether path is an existing directory. A missing path
// is not an error.
func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}

	return info.IsDir(), nil
}

// removeStaleTemp deletes temp files a crashed run left in target.
func removeStaleTemp(target string) error {
	stale, err := findStaleTemp(target)
	if err != nil || len(stale) == 0 {
		return err
	}

	root, err := os.OpenRoot(target)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer root.Close()

	for _, rel := range stale {
		if err := root.Remove(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale %s: %w", rel, err)
		}
	}

	return nil
}

// findStaleTemp lists .robbe-tmp-* files below target. A missing target
// yields no files.
func findStaleTemp(target string) ([]string, error) {
	var stale []string

	err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == target && errors.Is(err, os.ErrNotExist) {
				return filepath.SkipAll
			}

			return err
		}

		if d.IsDir() || !strings.HasPrefix(d.Name(), tmpPrefix) {
			return nil
		}

		rel, err := filepath.Rel(target, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}

		stale = append(stale, rel)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan target: %w", err)
	}

	return stale, nil
}

// copyFile writes src to rel below root (an os.Root keeps the path inside
// the staging directory).
func copyFile(root *os.Root, src, rel string) error {
	if err := root.MkdirAll(filepath.Dir(rel), dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(rel), err)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	if err := root.WriteFile(rel, data, filePerm); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}

	return nil
}
