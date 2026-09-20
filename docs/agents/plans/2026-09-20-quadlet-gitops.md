---
date: 2026-09-20T15:05:43Z
git_commit: 61cdf53ac86671feeaa8c96e2a3e3035611d53fa
branch: main
topic: "GitOps for Podman Quadlet"
tags: [plan, cmd, config, app, ports, adapters, deploy, quadlet, systemd, go-git]
status: ready
---

# PLAN: GitOps for Podman Quadlet (robbe)

Build `robbe`, a small Go CLI that keeps a host's Podman Quadlet units in sync with a git repository. A systemd timer runs `robbe sync` periodically; each run fetches the repo, computes the desired set of quadlet files for this host, validates them with the quadlet generator, writes them into a directory owned by robbe, and starts, restarts or stops the affected systemd units. No daemon, no Kubernetes, no `.kube` units.

## Acceptance Criteria

- `robbe sync` clones or fetches the configured repository with go-git (ssh key file, ssh agent, or HTTP token) and checks out the configured `ref` (branch or tag).
- Layout resolution: if `hosts/` exists in the repo, the desired tree is `common/` overlaid by `hosts/<hostname>/`, host file winning on equal relative path. If `hosts/` is absent, the repo root is the desired tree (flat, single host). `common/` is optional. If `hosts/` exists but `hosts/<hostname>/` does not, only `common/` is applied and a warning is logged.
- Desired tree is written only into `<quadlet dir>/robbe/`. Files outside that subdirectory are never read, written or deleted.
- Before the target directory is touched, the staged tree must pass `podman-system-generator --dryrun` (via `QUADLET_UNIT_DIRS`). On failure robbe aborts with a non-zero exit code and the target is unchanged.
- Diff between staged tree and target drives systemctl: removed unit file → `stop` then delete; added unit file → `start`; changed unit file → `restart`; any added, changed or removed non-unit file (env file, config, drop-in) → `restart` of all managed units. `daemon-reload` runs after files are written and before any start/restart.
- Unit names come from the generator dry-run output (`---name.service---` blocks with `SourcePath=`), so `ServiceName=` and every quadlet type (`.container`, `.pod`, `.volume`, `.network`, `.image`, `.build`) are handled by podman, not by robbe.
- `<target>/.robbe-commit` holds two lines: commit hash, then RFC3339 apply time. If the fetched commit equals the first line, sync exits 0 without diffing or touching systemd.
- `robbe sync --dry-run` fetches, prints the plan (files to add/change/remove, units to start/restart/stop) and changes nothing on disk or in systemd.
- `robbe status` prints applied commit, remote HEAD for `ref`, and drift (files in target that differ from the desired tree).
- `robbe install [--interval 5m]` writes `robbe-sync.service` (oneshot) and `robbe-sync.timer` to `~/.config/systemd/user/` (non-root) or `/etc/systemd/system/` (root), runs `daemon-reload` and `enable --now robbe-sync.timer`. `robbe uninstall` disables the timer and removes both files. Non-root install warns when linger is not enabled for the user.
- Configuration is read from yaml (`/etc/robbe/.robbe.yaml`, `~/.robbe.yaml`, `./.robbe.yaml`) and `ROBBE_*` environment variables. Target and cache directories default by uid.
- `go build ./...`, `golangci-lint run`, and `make test` pass. Planner and applier logic are covered by unit tests using a local git repository created in a temp dir and a fake `systemctl` / generator.

## Technical Key Decisions and Tradeoffs

1. **Run model: one-shot `sync` driven by a systemd timer.** No long-running process.
   - Why: no scheduler, retry or backoff code; timer state visible in `systemctl list-timers`; a crash never leaves a daemon dead.
   - Impact: every run must be idempotent and cheap when nothing changed (`.robbe-commit` short-circuit).
2. **Git access via `github.com/go-git/go-git/v5` (v5.19.x).** No `git` binary on the host.
   - Why: user decision; removes a runtime dependency; v6 is still alpha.
   - Impact: robbe implements auth selection itself: `ssh_key` file → `ssh.NewPublicKeysFromFile`, no key and ssh URL → `ssh.NewSSHAgentAuth`, `token` → `http.BasicAuth`. Known-hosts handling uses go-git defaults (`~/.ssh/known_hosts`).
3. **Repository layout: `hosts/<hostname>/` overlaying `common/`, flat fallback.**
   - Why: one repo can serve many hosts while a single-host repo stays trivial.
   - Impact: a pure `layout` resolver function with the merge rule "host wins on equal relative path"; hostname from `os.Hostname()` unless `host` is configured.
4. **Ownership by dedicated subdirectory `<quadlet dir>/robbe/`.**
   - Why: quadlet searches subdirectories recursively (podman-systemd.unit(5), "unit files placed in subdirectories"), so unit names are unaffected; prune is "anything in the subdir that is not desired"; no state file or ownership markers.
   - Impact: the only extra state is `<target>/.robbe-commit`. Manually placed quadlets outside the subdir are never touched.
5. **Validate with the real generator before applying.** `QUADLET_UNIT_DIRS=<staging> podman-system-generator [--user] --dryrun`.
   - Why: verified locally: exit code 1 and an error line on an invalid key, exit 0 with one `---name.service---` block per unit on success. Fails before the host is broken.
   - Impact: the same call yields the source-file → unit-name mapping; robbe does not reimplement quadlet naming rules. Generator path defaults to `/usr/lib/systemd/system-generators/podman-system-generator`, configurable.
6. **Restart policy: restart on change, support-file change restarts all managed units.**
   - Why: gitops semantics require the running state to follow the repo; per-reference tracking of env/config files is not worth the code in the first version.
   - Impact: simple rule in the planner; documented in the README.
7. **CLI surface: `sync`, `sync --dry-run`, `status`, `install`, `uninstall`, `version`.**
8. **Deployment: native binary, systemd oneshot service + timer.** Reference units shipped in `deploy/systemd/`, and `robbe install` writes the same units. `deploy/helm/` is removed; the Containerfile build path is fixed but the container image is not the primary deployment path.
   - Why: robbe needs `systemctl`, the quadlet directory and the generator on the host; containerizing it would need dbus and systemd mounts.
9. **Hexagonal layout as already scaffolded.** `ports/` defines `Source` (git), `Validator` (generator), `UnitManager` (systemctl), `Clock`-free; `adapters/` holds `gogit`, `generator`, `systemctl`; `app/` holds the planner and applier and depends only on `ports/`.
10. **Process execution through a single `exec`-style port** so tests inject fake commands instead of shimming `PATH`.

## Current State

Fresh scaffold, no domain code. Template rename is half done.

```
robbe/  (module github.com/m4schini/robbe, go 1.26, cobra + viper + zap)
├── main.go               calls cmd.Execute(), sets config.Version
├── cmd/root.go           placeholder cobra root, Use = config.AppName ("myproject")
├── config/config.go      viper paths: ~/.<app>.yaml, /etc/<app>/, ./
├── config/defaults.go    AppName = "myproject"
├── telemetry/logger.go   imports "myproject/config"  -> build broken
├── adapters/ app/ ports/ empty
├── deploy/helm/README    placeholder (no Kubernetes wanted)
├── deploy/quadlet/README placeholder
├── Containerfile         builds ./cmd/app, but main.go is at the module root
├── Makefile              test / test-integration (tag: integration)
└── .golangci.yml         all linters on, few disabled
```

Host environment verified: podman 6.1.2, generator at `/usr/lib/systemd/system-generators/podman-system-generator` with flags `-dryrun`, `-user`; `QUADLET_UNIT_DIRS` overrides the search path.

## Desired End State

```
                 ┌──────────────┐
 timer ──────▶   │  robbe sync  │
                 └──────┬───────┘
                        │
   ┌────────────────────▼───────────────────────┐
   │ 1. gogit: clone/fetch <cache>/repo, checkout ref, HEAD=abc123
   │ 2. HEAD == <target>/.robbe-commit ? exit 0
   │ 3. layout: common/ + hosts/<host>/  (or repo root)  -> desired tree
   │ 4. write desired tree to <cache>/staging, generator --dryrun
   │      -> error? abort, exit 1        -> ok: source->unit map
   │ 5. diff staging vs <target>          -> plan
   │ 6. stop removed units, delete files
   │ 7. write added/changed files
   │ 8. systemctl daemon-reload
   │ 9. start added units, restart changed units
   │10. write <target>/.robbe-commit
   └────────────────────────────────────────────┘

<target> = ~/.config/containers/systemd/robbe   (uid != 0)
           /etc/containers/systemd/robbe         (uid == 0)
```

Repository layouts accepted:

```
multi-host                       single host
repo/                            repo/
├── common/                      ├── nginx.container
│   └── proxy.network            ├── nginx.env
└── hosts/                       └── web.network
    ├── alpha/
    │   ├── nginx.container
    │   └── nginx.env
    └── beta/
        └── db.container
```

CLI mockups:

```
$ robbe sync --dry-run
repo    git@github.com:me/infra.git ref=main
commit  abc1234 (applied: 9f8e7d6)
host    alpha  layout=hosts (common + hosts/alpha)
target  /home/aurora/.config/containers/systemd/robbe

files
  + hosts/alpha/nginx.container
  ~ common/proxy.network
  - db.container
  ~ hosts/alpha/nginx.env          (support file -> restart all)

units
  stop     db.service
  start    nginx.service
  restart  proxy-network.service
dry run: no changes made

$ robbe status
applied  9f8e7d6  2026-09-20T14:02:11Z
remote   abc1234  main
drift    none
```

## Abstractions and Code Reuse

Existing: cobra root command, viper config loading (`config.Init`), zap logger (`telemetry.Logger`). Reused as-is after the rename fix.

New:

- `ports/`
  - `source.go` - `Source` interface: `Sync(ctx, url, ref string, auth Auth) (Checkout, error)`; `Checkout{Dir string; Commit string}`; `RemoteHead(ctx, url, ref string, auth Auth) (string, error)`; `Auth{SSHKey, SSHKeyPassword, Token, Username string}` (config maps onto it).
  - `validator.go` - `Validator` interface: `Validate(ctx, dir string, user bool) (map[string]string, error)` returning source path → unit name.
  - `units.go` - `UnitManager` interface: `DaemonReload`, `Start`, `Restart`, `Stop` (each `(ctx, unit string) error`, scope user/system chosen at construction).
  - `exec.go` - `Runner` interface: `Run(ctx, name string, env []string, args ...string) (stdout, stderr []byte, err error)`; single seam for tests.
- `adapters/`
  - `gogit/source.go` - implements `Source` with go-git: `PlainClone` on first run, `Fetch` + `Worktree.Checkout(Hash)` after; `ResolveRevision("refs/remotes/origin/<ref>")`; auth from `ports.Auth`.
  - `generator/validator.go` - implements `Validator` via `Runner`, parses `---X.service---` and `SourcePath=` lines, returns error with generator stderr when exit code != 0.
  - `systemctl/units.go` - implements `UnitManager` via `Runner`, adds `--user` for non-root.
  - `osexec/runner.go` - implements `Runner` with `os/exec`.
- `app/`
  - `layout/layout.go` - `Resolve(repoDir, host string) (Tree, Layout, error)`; `Tree = map[relPath]absSourcePath`.
  - `plan/plan.go` - `File{Rel string; Unit string}` (`Unit` empty for support files); `Diff(desired Tree, targetDir string, units map[string]string) (Plan, error)`; `Plan{Add, Change, Remove []File; Start, Restart, Stop []string; RestartAll bool}`; `Plan.String()` for `--dry-run`.
  - `sync/sync.go` - `Syncer{Source, Validator, Units, Cfg}`; `Result{Commit string; Layout layout.Layout; Plan plan.Plan; Applied bool}`; `Run(ctx, dryRun bool) (Result, error)` implements the ten steps.
  - `status/status.go` - `Status(ctx)` combining applied commit, remote head, drift.
  - `install/install.go` - unit file templates, `Install(interval)`, `Uninstall()`.
- `config/`
  - `config.go` - `Config` struct bound to viper keys; `Load() (Config, error)`; defaults for target, cache, generator path, scope by `os.Geteuid()`.
- `cmd/`
  - `sync.go`, `status.go`, `install.go`, `uninstall.go`, `version.go` - thin cobra wrappers building adapters and calling `app`.

Test helpers: `internal/testutil/runner.go` exports `FakeRunner` (implements `ports.Runner`, records calls, returns scripted output per command); `internal/testutil/repo.go` exports `NewRepo(t)` building a local git repo with go-git and committing files. Both are non-`_test` files so every package can import them.

## Logging & Observability

zap structured logs to stderr, production config unless `DEVELOPMENT=1`. Exit code 0 on success or no-op, 1 on any error. Example lines:

```
INFO  sync  fetched         url=git@github.com:me/infra.git ref=main commit=abc1234
INFO  sync  no changes      commit=abc1234
INFO  sync  layout          mode=hosts host=alpha common=true
WARN  sync  host dir missing host=alpha applying=common
INFO  sync  validated       units=3 dir=/home/aurora/.cache/robbe/staging
INFO  sync  plan            add=1 change=1 remove=1 start=1 restart=1 stop=1
INFO  sync  unit            action=restart unit=proxy-network.service
ERROR sync  validation failed err="converting \"bad.container\": unsupported key 'Img' in group 'Container'"
```

## Implementation

### Phase 0: Finish the scaffold rename

Dependencies: None

Make the module build and lint, remove template leftovers, add `robbe version`.

**Tasks**:
- [x] `config/defaults.go`: `AppName = "robbe"`.
- [x] `telemetry/logger.go`: import `github.com/m4schini/robbe/config`.
- [x] `cmd/root.go`: replace placeholder `Short`/`Long` with a one-line description of robbe; `SilenceUsage: true`.
- [x] `cmd/version.go`: `version` subcommand printing `config.Version`.
- [x] `Containerfile`: build path `./cmd/app` → `.`; label title `robbe`; remove `EXPOSE 8080`.
- [x] Remove `deploy/helm/`; replace `deploy/quadlet/README` with `deploy/systemd/README.md` stating that robbe runs natively (units added in Phase 4).
- [x] `README.md`: project name, one-paragraph purpose, layout tree updated (helm removed, systemd added).
- [x] `.gitignore`: ensure `.idea/` is ignored.
- [x] `go mod tidy`.

**Automated Verification**:
- [x] `go build ./...` succeeds.
- [x] `go run . version` prints `dev`.
- [x] `golangci-lint run ./...` passes.
- [x] `make test` passes.

### Phase 1: Configuration and git source (`robbe status` without drift)

Dependencies: Phase 0

Load config, fetch the repository with go-git, and expose `robbe status` showing applied commit and remote HEAD.

**Tasks**:
- [x] `go get github.com/go-git/go-git/v5@v5.19.2`.
- [x] `config/config.go`: `Config` struct with fields `Repo{URL, Ref string; Auth{SSHKey, SSHKeyPassword, Token, Username string}}`, `Host`, `Target`, `Cache`, `Generator`, `User bool`; `Load()` sets defaults (`Ref: main`, `Host: os.Hostname()`, `User: euid != 0`, `Target`, `Cache: ~/.cache/robbe` or `/var/cache/robbe`, `Generator` path) and validates `Repo.URL` non-empty. Bind `ROBBE_` env prefix with `_` key replacer.
- [x] `ports/source.go`: `Source` interface, `Checkout` struct, `Auth` value type.
- [x] `adapters/gogit/source.go`: auth selection (`ssh_key` → `NewPublicKeysFromFile`, ssh URL without key → `NewSSHAgentAuth("git")`, `token` → `http.BasicAuth{Username: username or "git", Password: token}`); `Sync` does `PlainClone` into `<cache>/repo` if missing else `Fetch` (`RefSpecs: +refs/heads/*:refs/remotes/origin/*`, `+refs/tags/*:refs/tags/*`, `Force: true`), resolves `refs/remotes/origin/<ref>` then `refs/tags/<ref>`, `Checkout{Hash, Force: true}`; `RemoteHead` uses `Fetch` + resolve without checkout.
- [x] `adapters/gogit/source_test.go`: using `testutil.NewRepo(t)` (`internal/testutil/repo.go`, created here) build a repo in `t.TempDir()`, commit two files on `main`, tag `v1`; assert `Sync` checks out branch, then tag, then a new commit after re-push.
- [x] `app/status/status.go`: `Status{Applied, AppliedAt, Remote string}`; `ReadMarker(target) (commit string, at time.Time, err)` parses the two-line `.robbe-commit` (absent → `none`); `WriteMarker(target, commit, at)` counterpart; both reused by `app/sync` in Phases 2 and 3.
- [x] `cmd/status.go`: prints the mockup table (drift line added in Phase 2).
- [x] `docs/configuration.md`: every config key, env variable name, default, example yaml.

**Automated Verification**:
- [x] `go test ./adapters/gogit/... ./config/...` passes (`TestSync_Branch`, `TestSync_Tag`, `TestSync_FetchNewCommit`, `TestLoad_Defaults`, `TestLoad_EnvOverride`).
- [x] `ROBBE_REPO_URL=<local bare repo> go run . status` prints `applied none` and a remote commit.
- [x] `golangci-lint run ./...` passes.

### Phase 2: Layout, validation, plan (`robbe sync --dry-run`)

Dependencies: Phase 1

Resolve the desired tree, validate it with the generator, diff against the target and print the plan. Nothing is written to the target.

**Tasks**:
- [x] `app/layout/layout.go`: `Resolve(repoDir, host)`; walk `common/` then `hosts/<host>/` (later overrides), or repo root when `hosts/` absent; `hosts/` present without `hosts/<host>/` → apply `common/` only and set `HostDirFound=false` (caller logs the warning); `hosts/` present, no `common/`, no `hosts/<host>/` → empty tree (everything pruned); skip `.git`, hidden files, and any `*.kube` file (logged as unsupported); return `Tree` and `Layout{Mode, HostDirFound, CommonFound}`.
- [x] `app/layout/layout_test.go`: flat, hosts+common, host wins, host dir missing, hosts without common.
- [x] `ports/exec.go`, `adapters/osexec/runner.go`, plus `FakeRunner` in `internal/testutil/runner.go`; reuse `testutil.NewRepo` from Phase 1.
- [x] `ports/validator.go`, `adapters/generator/validator.go`: runs `<generator> [-user] -dryrun` with `QUADLET_UNIT_DIRS=<dir>`; parses stdout for `^---(.+)---$` and `^SourcePath=(.+)$`; returns `map[relPath]unitName` (rel to `dir`); non-zero exit → error wrapping stderr/stdout lines containing `converting`.
- [x] `adapters/generator/validator_test.go`: table test with recorded generator output (valid, invalid key, two units, `ServiceName=` override) via `FakeRunner`; integration test (`//go:build integration`) running the real generator when present.
- [x] `app/plan/plan.go`: `Diff(desired Tree, targetDir, units)`; classify each rel path as unit file (`.container|.pod|.volume|.network|.image|.build`) or support file; compare bytes; fill `Add/Change/Remove`, `Start/Restart/Stop`; `RestartAll` when any support file differs, then `Restart` = all units in desired minus `Start`; deterministic ordering (sorted). `Plan.Empty()`, `Plan.String()`.
- [x] `app/plan/plan_test.go`: add/change/remove/unchanged, support-file change restarts all, removed unit stopped, `.robbe-commit` ignored in target.
- [x] `app/sync/sync.go`: `Syncer.Run(ctx, dryRun)` steps 1–5 (fetch, short-circuit when `status.ReadMarker` commit equals HEAD, layout, stage into `<cache>/staging` (recreated each run), validate, diff); dry-run stops here and returns `Result{Plan, Commit, Layout}`.
- [x] `cmd/sync.go`: `sync` command with `--dry-run`, prints the mockup output.
- [x] `cmd/status.go`: add drift line from `plan.Diff` (`none` or counts).
- [x] `docs/layout.md`: repository layouts, merge rule, restart rule, unsupported `.kube`.

**Automated Verification**:
- [x] `go test ./app/... ./adapters/generator/...` passes.
- [x] `go test -tags=integration ./adapters/generator/...` passes on a host with podman.
- [x] `ROBBE_REPO_URL=<local repo> ROBBE_TARGET=$(mktemp -d) go run . sync --dry-run` prints a plan and leaves the target empty.
- [x] `golangci-lint run ./...` passes.

### Phase 3: Apply (`robbe sync`)

Dependencies: Phase 2

Write files and drive systemd.

**Tasks**:
- [x] `ports/units.go`, `adapters/systemctl/units.go`: `systemctl [--user] daemon-reload|start|restart|stop <unit>`; errors include stderr.
- [x] `app/sync/sync.go`: steps 6–10: stop removed units (continue on error, collect), delete removed files and empty parent dirs, write added/changed files (create dirs, `0644`, atomic via temp + rename), `DaemonReload`, start added, restart changed (or all when `RestartAll`), write `.robbe-commit` via `status.WriteMarker` only when every unit action succeeded; on any unit action failure leave the marker unchanged so the next run retries, and return the joined error.
- [x] `app/sync/sync_test.go`: end-to-end with local bare repo, temp target, `FakeRunner` scripted for generator and systemctl; asserts file contents in target, exact systemctl call sequence (`stop` before delete, `daemon-reload` before `start`), short-circuit on second run, failed validation leaves target untouched, unit failure keeps old `.robbe-commit`.
- [x] `cmd/sync.go`: non-dry-run path prints applied summary; exit 1 on error.
- [x] `Makefile`: `lint` target (`golangci-lint run`), `build` target with `-ldflags -X main.version`.

**Automated Verification**:
- [x] `go test ./...` passes (`TestSync_Apply`, `TestSync_NoOpOnSameCommit`, `TestSync_ValidationFailureLeavesTarget`, `TestSync_RemovedUnitStopped`, `TestSync_SupportFileRestartsAll`).
- [x] `golangci-lint run ./...` passes.

**Manual Verification**:
- [ ] On a rootless podman host: point `.robbe.yaml` at a repo with one `.container`, run `robbe sync`, `systemctl --user status <name>.service` is active.
- [ ] Change the image tag in the repo, run `robbe sync`, unit restarted with new image.
- [ ] Delete the `.container` in the repo, run `robbe sync`, unit stopped and file gone from `~/.config/containers/systemd/robbe/`.

### Phase 4: Install command and deployment units

Dependencies: Phase 3

Make robbe run itself on a timer.

**Tasks**:
- [x] `deploy/systemd/robbe-sync.service`: `Type=oneshot`, `ExecStart=/usr/local/bin/robbe sync`; `deploy/systemd/robbe-sync.timer`: `OnBootSec=1min`, `OnUnitActiveSec=5min`, `Persistent=true`, `WantedBy=timers.target`; `deploy/systemd/README.md` with user and system install steps and `loginctl enable-linger`.
- [x] `app/install/install.go`: embed both unit templates (`embed.FS`), render with binary path (`os.Executable()`) and interval; `Install(interval)` writes to `~/.config/systemd/user/` or `/etc/systemd/system/`, `daemon-reload`, `enable --now robbe-sync.timer`; `Uninstall()` runs `disable --now`, removes files, `daemon-reload`. Linger check: `loginctl show-user <uid> -p Linger` → warn when `Linger=no`.
- [-] `app/install/install_test.go`: rendered unit content, file locations by scope, systemctl call sequence via `FakeRunner`.
- [x] `cmd/install.go`, `cmd/uninstall.go`: `--interval` flag (default `5m`, `time.Duration`).
- [x] `README.md`: quick start (build, config, `robbe install`), command reference, link to `docs/configuration.md` and `docs/layout.md`.
- [x] `.github/workflows/ci.yml`: no change needed; confirm integration action runs `make test-integration` and tolerates missing generator (tests skip when binary absent).

**Automated Verification**:
- [ ] `go test ./app/install/...` passes.
- [ ] `go run . install --interval 1m` with `HOME=$(mktemp -d)` and `FakeRunner`-free path is not testable; covered by unit tests above.
- [ ] `golangci-lint run ./...` and `make test` pass.

**Manual Verification**:
- [ ] `robbe install` on a rootless host: `systemctl --user list-timers` shows `robbe-sync.timer`; journal shows a sync run after the interval.
- [ ] `robbe uninstall` removes the timer and both unit files.

## Implementation Notes

During implementation, document user feedback, problems, and decisions here.

- Phase 0: `exhaustruct_v5` rejected every `cobra.Command` literal (about 40 fields). Added `ignore-patterns: ['.+/cobra\.Command$']` in `.golangci.yml` instead of disabling the linter.

## References

- podman-systemd.unit(5): unit search paths and recursive subdirectories, `QUADLET_UNIT_DIRS` debugging section, unit naming per type.
- go-git v5: https://pkg.go.dev/github.com/go-git/go-git/v5 (`PlainClone`, `Repository.Fetch`, `Worktree.Checkout`, `plumbing/transport/ssh`, `plumbing/transport/http`).
- systemd.timer(5), systemd.service(5) `Type=oneshot`.
- Verified locally 2026-09-20: `QUADLET_UNIT_DIRS=<dir> podman-system-generator --user --dryrun` exits 1 on `unsupported key`, prints `---t.service---` blocks with `SourcePath=` on success (podman 6.1.2).
