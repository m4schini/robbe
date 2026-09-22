// SPDX-License-Identifier: TODO

package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/m4schini/robbe/adapters"
	"github.com/m4schini/robbe/adapters/generator"
	"github.com/m4schini/robbe/adapters/gogit"
	"github.com/m4schini/robbe/adapters/systemctl"
	"github.com/m4schini/robbe/app/lock"
	"github.com/m4schini/robbe/app/marker"
	"github.com/m4schini/robbe/internal/testutil"
	"github.com/m4schini/robbe/quadlet"
)

const (
	genBin    = "/fake/podman-system-generator"
	nginx     = "[Container]\nImage=docker.io/library/nginx:1\n"
	nginxV2   = "[Container]\nImage=docker.io/library/nginx:2\n"
	proxyNet  = "[Network]\n"
	badUnit   = "[Container]\nImg=x\n"
	appEnv    = "A=1\n"
	appEnvV2  = "A=2\n"
	stateDown = "inactive"
)

// fakeHost emulates the generator and systemctl behind a FakeRunner: the
// generator derives unit names from the files in the validated dir, and
// systemctl keeps a unit state table.
type fakeHost struct {
	mu        sync.Mutex
	states    map[string]string
	failStart map[string]bool
	failStop  map[string]bool
}

func newFakeHost() *fakeHost {
	return &fakeHost{mu: sync.Mutex{}, states: map[string]string{}, failStart: map[string]bool{}, failStop: map[string]bool{}}
}

func (h *fakeHost) handle(c testutil.Call) (testutil.Result, bool) {
	switch c.Name {
	case genBin:
		return h.generate(c), true
	case "systemctl":
		return h.systemctl(c), true
	default:
		return testutil.Result{Stdout: "", Stderr: "", Err: nil}, false
	}
}

func (h *fakeHost) generate(c testutil.Call) testutil.Result {
	var dir string

	for _, e := range c.Env {
		dir = strings.TrimPrefix(e, "QUADLET_UNIT_DIRS=")
	}

	var stdout, stderr strings.Builder

	failed := false

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(dir, path)
		if !quadlet.IsUnitFile(rel) {
			return nil
		}

		content, _ := os.ReadFile(path)
		if strings.Contains(string(content), "Img=") {
			failed = true
			fmt.Fprintf(&stderr, "quadlet-generator[1]: converting %q: unsupported key 'Img' in group 'Container' in %s\n", rel, path)

			return nil
		}

		unit := quadlet.ConventionalUnit(rel)

		for line := range strings.SplitSeq(string(content), "\n") {
			if name, ok := strings.CutPrefix(line, "ServiceName="); ok {
				unit = name + ".service"
			}
		}

		fmt.Fprintf(&stdout, "---%s---\n[Unit]\nSourcePath=%s\n\n", unit, path)

		return nil
	})

	if failed {
		stderr.WriteString("quadlet-generator[1]: processing encountered some errors\n")

		return testutil.Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: testutil.ErrExit}
	}

	return testutil.Result{Stdout: stdout.String(), Stderr: "", Err: nil}
}

func (h *fakeHost) systemctl(c testutil.Call) testutil.Result {
	h.mu.Lock()
	defer h.mu.Unlock()

	args := c.Args
	if len(args) > 0 && args[0] == "--user" {
		args = args[1:]
	}

	verb, units := args[0], args[1:]

	var (
		failed []string
		out    strings.Builder
	)

	for _, u := range units {
		switch verb {
		case "start", "restart":
			if h.failStart[u] {
				h.states[u] = "failed"
				failed = append(failed, u)
			} else {
				h.states[u] = "active"
			}
		case "stop":
			if h.failStop[u] {
				failed = append(failed, u)
			} else {
				h.states[u] = stateDown
			}
		case "is-active":
			state, ok := h.states[u]
			if !ok {
				state = stateDown
			}

			if state != "active" {
				failed = append(failed, u)
			}

			out.WriteString(state + "\n")
		}
	}

	if len(failed) > 0 {
		return testutil.Result{Stdout: out.String(), Stderr: "Job for " + strings.Join(failed, ", ") + " failed.", Err: testutil.ErrExit}
	}

	return testutil.Result{Stdout: out.String(), Stderr: "", Err: nil}
}

func (h *fakeHost) state(unit string) string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.states[unit]; ok {
		return s
	}

	return stateDown
}

// harness bundles a repo, a fake host and a Syncer on a temp target.
type harness struct {
	repo    *testutil.Repo
	host    *fakeHost
	runner  *testutil.FakeRunner
	syncer  *Syncer
	target  string
	state   string
	runtime string
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	repo := testutil.NewRepo(t)
	host := newFakeHost()
	runner := testutil.NewFakeRunner()
	runner.Handler = host.handle

	target := filepath.Join(t.TempDir(), "robbe")
	cache := t.TempDir()
	state := filepath.Join(t.TempDir(), "robbe")
	runtime := filepath.Join(t.TempDir(), "robbe")

	var noAuth adapters.Auth

	syncer := &Syncer{
		Source:    gogit.New(cache),
		Validator: generator.New(genBin, runner),
		Units:     systemctl.New(runner, true),
		Opts: Options{
			URL:        repo.URL(),
			Ref:        "main",
			Auth:       noAuth,
			Host:       "alpha",
			Target:     target,
			Cache:      cache,
			State:      state,
			Runtime:    runtime,
			User:       true,
			AllowEmpty: false,
		},
		Log: nil,
		Now: func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) },
	}

	return &harness{repo: repo, host: host, runner: runner, syncer: syncer, target: target, state: state, runtime: runtime}
}

func (h *harness) run(t *testing.T, dryRun bool) Result {
	t.Helper()

	res, err := h.syncer.Run(t.Context(), dryRun)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	return res
}

// systemctlLines returns the recorded systemctl calls without the --user flag.
func (h *harness) systemctlLines() []string {
	var lines []string

	for _, c := range h.runner.Calls() {
		if c.Name == "systemctl" {
			lines = append(lines, strings.TrimPrefix(c.String(), "systemctl --user "))
		}
	}

	return lines
}

func (h *harness) targetFiles(t *testing.T) map[string]string {
	t.Helper()

	files := map[string]string{}

	err := filepath.WalkDir(h.target, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(h.target, path)
		data, _ := os.ReadFile(path)
		files[rel] = string(data)

		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("walk target: %v", err)
	}

	return files
}

func (h *harness) marker(t *testing.T) marker.Marker {
	t.Helper()

	m, err := marker.Read(h.state)
	if err != nil {
		t.Fatalf("marker: %v", err)
	}

	return m
}

func assertEqual[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("systemctl calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSync_Apply(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	commit := h.repo.Commit("init", map[string]string{"nginx.container": nginx, "app.env": appEnv, "proxy.network": proxyNet})

	res := h.run(t, false)

	assertEqual(t, "applied", res.Applied, true)
	assertEqual(t, "commit", res.Commit, commit)
	assertEqual(t, "previous", res.Previous, "")

	files := h.targetFiles(t)
	assertEqual(t, "nginx.container", files["nginx.container"], nginx)
	assertEqual(t, "app.env", files["app.env"], appEnv)
	assertEqual(t, "proxy.network", files["proxy.network"], proxyNet)
	assertEqual(t, "marker", h.marker(t).Commit, commit)
	assertEqual(t, "marker time", h.marker(t).At, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))

	// The marker lives in the state dir; the target holds quadlet content only.
	if _, err := os.Stat(filepath.Join(h.state, marker.File)); err != nil {
		t.Errorf("marker not in state dir: %v", err)
	}

	for rel := range files {
		if strings.HasPrefix(filepath.Base(rel), ".") {
			t.Errorf("target holds robbe file %q, want quadlet content only", rel)
		}
	}

	assertLines(t, h.systemctlLines(), []string{
		"daemon-reload",
		"restart nginx.service proxy-network.service",
		"is-active nginx.service proxy-network.service",
	})
	assertEqual(t, "nginx state", h.host.state("nginx.service"), "active")

	// The generator ran on the staging dir with the user flag.
	var genCalls []string

	for _, c := range h.runner.Calls() {
		if c.Name == genBin {
			genCalls = append(genCalls, strings.Join(c.Args, " ")+" "+strings.Join(c.Env, " "))
		}
	}

	if len(genCalls) != 1 || !strings.HasPrefix(genCalls[0], "-user -dryrun QUADLET_UNIT_DIRS="+filepath.Join(h.syncer.Opts.Cache, "staging")) {
		t.Errorf("generator calls = %q", genCalls)
	}
}

func TestSync_NoOpOnSameCommit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	res := h.run(t, false)

	assertEqual(t, "up to date", res.UpToDate, true)
	assertEqual(t, "applied", res.Applied, false)
	assertLines(t, h.systemctlLines(), nil)
}

func TestSync_TargetWipedReapplies(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	commit := h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	if err := os.RemoveAll(h.target); err != nil {
		t.Fatal(err)
	}

	res := h.run(t, false)

	assertEqual(t, "up to date", res.UpToDate, false)
	assertEqual(t, "applied", res.Applied, true)
	assertEqual(t, "previous", res.Previous, commit)
	assertEqual(t, "nginx.container", h.targetFiles(t)["nginx.container"], nginx)
	assertEqual(t, "marker", h.marker(t).Commit, commit)
	assertLines(t, h.systemctlLines(), []string{
		"daemon-reload",
		"restart nginx.service",
		"is-active nginx.service",
	})
}

func TestSync_DryRun(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx, "app.env": appEnv})

	res := h.run(t, true)

	assertEqual(t, "applied", res.Applied, false)
	assertEqual(t, "add", len(res.Plan.Add), 2)
	assertEqual(t, "start", strings.Join(res.Plan.Start, ","), "nginx.service")

	if _, err := os.Stat(h.target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("target exists after dry run: %v", err)
	}

	assertLines(t, h.systemctlLines(), nil)
}

func TestSync_ValidationFailureLeavesTarget(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	h.repo.Commit("break", map[string]string{"nginx.container": nginxV2, "bad.container": badUnit})

	_, err := h.syncer.Run(t.Context(), false)
	if !errors.Is(err, generator.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}

	if !strings.Contains(err.Error(), `converting "bad.container"`) {
		t.Errorf("err = %v, want generator message", err)
	}

	assertEqual(t, "nginx.container", h.targetFiles(t)["nginx.container"], nginx)
	assertEqual(t, "marker", h.marker(t).Commit, first)
	assertLines(t, h.systemctlLines(), nil)
}

func TestSync_ChangedUnitRestarted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx, "proxy.network": proxyNet})
	h.run(t, false)
	h.runner.Reset()

	commit := h.repo.Commit("bump", map[string]string{"nginx.container": nginxV2})
	res := h.run(t, false)

	assertEqual(t, "change", res.Plan.Change[0].Rel, "nginx.container")
	assertEqual(t, "nginx.container", h.targetFiles(t)["nginx.container"], nginxV2)
	assertEqual(t, "marker", h.marker(t).Commit, commit)
	assertLines(t, h.systemctlLines(), []string{
		"daemon-reload",
		"restart nginx.service",
		"is-active nginx.service",
	})
}

func TestSync_RemovedUnitStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx, "sub/db.container": nginx, "proxy.network": proxyNet})
	h.run(t, false)
	h.runner.Reset()

	h.repo.Remove("drop db", "sub/db.container")
	res := h.run(t, false)

	assertEqual(t, "stop", strings.Join(res.Plan.Stop, ","), "db.service")

	files := h.targetFiles(t)
	if _, ok := files["sub/db.container"]; ok {
		t.Error("sub/db.container still in target")
	}

	if _, err := os.Stat(filepath.Join(h.target, "sub")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty dir sub not pruned: %v", err)
	}

	assertEqual(t, "db state", h.host.state("db.service"), stateDown)
	assertLines(t, h.systemctlLines(), []string{
		"is-active nginx.service",
		"stop db.service",
		"is-active db.service",
		"daemon-reload",
	})
}

func TestSync_SupportFileRestartsAll(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx, "web.pod": "[Pod]\n", "proxy.network": proxyNet, "app.env": appEnv})
	h.run(t, false)
	h.runner.Reset()

	h.repo.Commit("env", map[string]string{"app.env": appEnvV2})
	res := h.run(t, false)

	assertEqual(t, "restart all", res.Plan.RestartAll, true)
	assertEqual(t, "restart", strings.Join(res.Plan.Restart, ","), "nginx.service,web-pod.service")
	assertEqual(t, "app.env", h.targetFiles(t)["app.env"], appEnvV2)
	assertLines(t, h.systemctlLines(), []string{
		"daemon-reload",
		"restart nginx.service web-pod.service",
		"is-active nginx.service web-pod.service",
	})
}

func TestSync_UnitFailureKeepsMarker(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	h.host.failStart["db.service"] = true
	h.repo.Commit("add db", map[string]string{"db.container": nginx})

	_, err := h.syncer.Run(t.Context(), false)
	if !errors.Is(err, ErrUnitsFailed) {
		t.Fatalf("err = %v, want ErrUnitsFailed", err)
	}

	if !strings.Contains(err.Error(), "db.service") {
		t.Errorf("err = %v, want failed unit named", err)
	}

	// Files are written, the marker is not.
	assertEqual(t, "db.container", h.targetFiles(t)["db.container"], nginx)
	assertEqual(t, "marker", h.marker(t).Commit, first)
}

func TestSync_RetriesInactiveUnit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)

	h.host.failStart["db.service"] = true
	h.repo.Commit("add db", map[string]string{"db.container": nginx})

	if _, err := h.syncer.Run(t.Context(), false); err == nil {
		t.Fatal("expected failure")
	}

	// The unit becomes startable and a commit lands that changes nothing in
	// the desired tree (.git* files are skipped by the layout).
	delete(h.host.failStart, "db.service")
	h.runner.Reset()

	commit := h.repo.Commit("ignore", map[string]string{".gitignore": "*.bak\n"})
	res := h.run(t, false)

	assertEqual(t, "file changes", len(res.Plan.Add)+len(res.Plan.Change)+len(res.Plan.Remove), 0)
	assertEqual(t, "start", strings.Join(res.Plan.Start, ","), "db.service")
	assertLines(t, h.systemctlLines(), []string{
		"is-active db.service nginx.service",
		"daemon-reload",
		"restart db.service",
		"is-active db.service",
	})
	assertEqual(t, "marker", h.marker(t).Commit, commit)
	assertEqual(t, "db state", h.host.state("db.service"), "active")
}

func TestSync_StopFailureKeepsFile(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.repo.Commit("init", map[string]string{"nginx.container": nginx, "db.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	h.host.failStop["db.service"] = true
	h.repo.Remove("drop db", "db.container")

	_, err := h.syncer.Run(t.Context(), false)
	if !errors.Is(err, ErrUnitsFailed) {
		t.Fatalf("err = %v, want ErrUnitsFailed", err)
	}

	assertEqual(t, "db.container kept", h.targetFiles(t)["db.container"], nginx)
	assertEqual(t, "marker", h.marker(t).Commit, first)
}

func TestSync_MovedFileRemoved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"a/x.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	// Same content at a new path: the unit is neither stopped nor restarted,
	// but the old file must still go so the target has one file per unit.
	h.repo.Remove("move", "a/x.container")
	commit := h.repo.Commit("move", map[string]string{"b/x.container": nginx})
	res := h.run(t, false)

	assertEqual(t, "add", res.Plan.Add[0].Rel, "b/x.container")
	assertEqual(t, "remove", res.Plan.Remove[0].Rel, "a/x.container")
	assertEqual(t, "stop", len(res.Plan.Stop), 0)

	files := h.targetFiles(t)
	assertEqual(t, "b/x.container", files["b/x.container"], nginx)

	if _, ok := files["a/x.container"]; ok {
		t.Error("a/x.container still in target")
	}

	assertEqual(t, "file count", len(files), 1)
	assertEqual(t, "marker", h.marker(t).Commit, commit)
}

func TestSync_EmptyTreeGuard(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"hosts/alpha/nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	// Host dir vanishes (e.g. hostname changed): everything would be pruned.
	h.repo.Remove("drop host", "hosts/alpha/nginx.container")
	h.repo.Commit("other host", map[string]string{"hosts/beta/db.container": nginx})

	_, err := h.syncer.Run(t.Context(), false)
	if !errors.Is(err, ErrEmptyTree) {
		t.Fatalf("err = %v, want ErrEmptyTree", err)
	}

	assertEqual(t, "nginx.container kept", h.targetFiles(t)["nginx.container"], nginx)
	assertLines(t, h.systemctlLines(), nil)

	h.syncer.Opts.AllowEmpty = true
	res := h.run(t, false)

	assertEqual(t, "layout host found", res.Layout.HostDirFound, false)
	assertEqual(t, "files left", len(h.targetFiles(t)), 0)
	assertEqual(t, "nginx state", h.host.state("nginx.service"), stateDown)
}

func TestSync_HostsLayout(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{
		"common/proxy.network":        proxyNet,
		"common/app.env":              appEnv,
		"hosts/alpha/nginx.container": nginx,
		"hosts/alpha/app.env":         appEnvV2,
		"hosts/beta/db.container":     nginx,
		"README.md":                   "x",
	})

	res := h.run(t, false)

	assertEqual(t, "mode", string(res.Layout.Mode), "hosts")

	files := h.targetFiles(t)

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	assertEqual(t, "files", strings.Join(keys, ","), "app.env,nginx.container,proxy.network")
	assertEqual(t, "host wins", files["app.env"], appEnvV2)
}

func TestSync_StaleTempRemoved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx})
	h.run(t, false)
	h.runner.Reset()

	stale := filepath.Join(h.target, ".robbe-tmp-123")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.repo.Commit("bump", map[string]string{"nginx.container": nginxV2})
	res := h.run(t, false)

	assertEqual(t, "restart all", res.Plan.RestartAll, false)

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale temp file not removed: %v", err)
	}
}

func TestSync_Locked(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx})

	unlock, err := lock.Acquire(h.runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	_, err = h.syncer.Run(t.Context(), false)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

func TestSync_LockDirCreated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repo.Commit("init", map[string]string{"nginx.container": nginx})

	if _, err := os.Stat(h.runtime); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir exists before the run: %v", err)
	}

	h.run(t, true)

	if _, err := os.Stat(filepath.Join(h.runtime, lock.FileName)); err != nil {
		t.Errorf("lock file not created under the runtime dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(h.syncer.Opts.Cache, lock.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock file still created in the cache: %v", err)
	}
}

func TestRemoveFile_TrailingSlashTarget(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "systemd")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(target, "sub", "a.container"), []byte(nginx), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tgt := range []string{target + "/", target + "//", target + "/./"} {
		if err := removeFile(tgt, "sub/a.container"); err != nil {
			t.Fatalf("removeFile(%q): %v", tgt, err)
		}
	}

	if _, err := os.Stat(filepath.Join(target, "sub")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty sub dir not pruned: %v", err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("target dir removed: %v", err)
	}

	if _, err := os.Stat(parent); err != nil {
		t.Errorf("target parent removed: %v", err)
	}
}
