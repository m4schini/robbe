---
date: 2026-09-21T06:27:07Z
git_commit: 1cbf7d9f705919aa4329881a65b15152abcb8ee0
branch: feat/quadlet-gitops
topic: "XDG Base Directory compliance"
tags: [plan, config, cmd, app, sync, marker, status, install, deploy, systemd, xdg]
status: ready
---

# PLAN: XDG Base Directory compliance

Move every file robbe owns to the location the XDG Base Directory Specification (0.8) prescribes: the config file to `$XDG_CONFIG_HOME/robbe/`, the applied-commit marker to `$XDG_STATE_HOME/robbe/`, the lock file to `$XDG_RUNTIME_DIR/robbe/`. Root runs use the system equivalents (`/etc/robbe`, `/var/lib/robbe`, `/run/robbe`, `/var/cache/robbe`). The systemd units declare the matching `*Directory=` directives so systemd creates the directories. The target (quadlet) directory and the cache directory are already spec-conformant and keep their defaults.

## Acceptance Criteria

- The config file is resolved in this order, first hit wins: the `--config` flag, `$XDG_CONFIG_HOME/robbe/config.yaml`, `<dir>/robbe/config.yaml` for each entry of `$XDG_CONFIG_DIRS` (default `/etc/xdg`), `/etc/robbe/config.yaml`. `~/.robbe.yaml` and `./.robbe.yaml` are no longer read.
- `--config <path>` pointing at a missing or unreadable file is an error; a missing file in the search path is not.
- An XDG variable that is empty or holds a relative path is treated as unset and the spec fallback applies (`~/.config`, `~/.cache`, `~/.local/state`, `/etc/xdg`).
- Root (euid 0) defaults: config `/etc/robbe`, cache `/var/cache/robbe`, state `/var/lib/robbe`, runtime `/run/robbe`, target `/etc/containers/systemd/robbe`.
- The marker lives at `<state>/applied`. New config key `state` (`ROBBE_STATE`), default `$XDG_STATE_HOME/robbe` or `/var/lib/robbe`. The target directory holds quadlet content only.
- `robbe sync` short-circuits ("up to date") only when the marker names the fetched commit and the target directory exists. A wiped target is re-applied.
- The lock lives at `<runtime>/lock`. New config key `runtime` (`ROBBE_RUNTIME`), default `$XDG_RUNTIME_DIR/robbe` or `/run/robbe`. When `XDG_RUNTIME_DIR` is unset for a non-root user, robbe logs a warning and uses `<cache>/lock`.
- `robbe status` reads the marker from the state directory.
- `app/install.UnitDir` and `config` share one XDG implementation in `internal/xdg`.
- Embedded unit template and `deploy/systemd/robbe-sync.service` carry `CacheDirectory=robbe`, `StateDirectory=robbe`, `RuntimeDirectory=robbe`, `ConfigurationDirectory=robbe`.
- `README.md`, `docs/configuration.md`, `docs/layout.md`, `deploy/systemd/README.md` describe the new locations and keys.
- `go build ./...`, `golangci-lint run ./...`, `make test` pass.

## Technical Key Decisions and Tradeoffs

1. **Config path: XDG search plus `/etc/robbe/config.yaml` fallback, legacy paths dropped.**
   - Why: robbe is unreleased (`main` has only the initial commits), so there is no compatibility cost. A dotfile in `$HOME` and a cwd lookup both violate the spec; the cwd lookup is also a foot-gun for a timer-driven tool.
   - Impact: `config.Init` rewritten around explicit paths, persistent `--config` flag on the root command, `viper.SetConfigName("config")`.
2. **Marker moves to the state directory.**
   - Why: the marker is state, the target is configuration owned by podman's convention. Keeping the target free of robbe files means "everything in the target is a quadlet file or support file".
   - Impact: new `state` key threaded through `sync.Options` and `status.Options`; `Syncer.Run` gains a target-exists guard before short-circuiting. The `.robbe` housekeeping prefix stays because atomic writes still create `.robbe-tmp-*` files inside the target; only the marker-related wording changes.
3. **Lock moves to the runtime directory with cache fallback.**
   - Why: a lock is per-boot runtime data. `XDG_RUNTIME_DIR` can be unset (cron, `sudo -u`, containers) and the spec asks for a warning plus a fallback rather than a failure. `/tmp` is rejected: world-writable, symlink games.
   - Impact: new `runtime` key; `Syncer.lock` takes the directory from `Options`; fallback resolution and the warning live in `cmd/sync.go`, so `app/sync` stays free of environment lookups.
4. **Hand-rolled `internal/xdg`, no dependency.**
   - Why: the root switch to `/etc`, `/var/lib`, `/run` is robbe-specific and no library does it. About 80 lines. Removes the duplicated `XDG_CONFIG_HOME` logic in `config` and `app/install`.
   - Impact: one place defines every default; tests cover the absolute-path rule and the root switch.
5. **systemd `*Directory=` directives in the units.**
   - Why: systemd creates the directories with the right owner and mode, for user units mapped onto the XDG directories. `/run/robbe` then exists for root runs without robbe creating directories under `/run`.
   - Impact: templates and reference units change; robbe still `MkdirAll`s for manual runs. `RuntimeDirectory` is removed when the oneshot unit stops, which is fine because the flock only lives as long as the process.

## Current State

```
                        user (euid != 0)                          root                         XDG?
config file   ~/.robbe.yaml  ->  /etc/robbe/.robbe.yaml  ->  ./.robbe.yaml   (both scopes)     no   dotfile in $HOME
target        $XDG_CONFIG_HOME/containers/systemd/robbe    /etc/containers/systemd/robbe       yes  (podman dictates)
cache         $XDG_CACHE_HOME/robbe                        /var/cache/robbe                    yes
  ├─ repo/      git clone
  ├─ staging/   validated tree, recreated each run
  └─ lock       flock file                                                                      no   runtime data in cache
marker        <target>/.robbe-commit                       <target>/.robbe-commit              no   state in config dir
unit dir      $XDG_CONFIG_HOME/systemd/user                /etc/systemd/system                 yes  (systemd dictates)
```

- `config/config.go:82-103` `Init()`: `os.UserHomeDir()` with `cobra.CheckErr`, `viper.AddConfigPath(home)`, `/etc/robbe`, `.`; `SetConfigName(".robbe")`; `ReadInConfig` error ignored.
- `config/config.go:123-148` `defaultTarget`, `defaultCache`: read `XDG_CONFIG_HOME` / `XDG_CACHE_HOME`, fall back to `~/.config` / `~/.cache`, switch on `user`.
- `app/install/install.go:61-71` `UnitDir(user, home)`: same `XDG_CONFIG_HOME` logic, duplicated.
- `cmd/install.go:73-84` `newInstaller`: calls `os.UserHomeDir()` only to feed `UnitDir`.
- `app/sync/sync.go:448-472` `lock()`: `MkdirAll(Cache)`, `<cache>/lock`, `flock(LOCK_EX|LOCK_NB)`, `ErrLocked` on `EWOULDBLOCK`.
- `app/sync/sync.go:111-123` `Run`: `marker.Read(Target)`, short-circuit when `applied.Commit == co.Commit`.
- `app/marker/marker.go`: `File = ".robbe-commit"`, `Read(target)`, `Write(target, m)` with `MkdirAll(target)`.
- `app/status/status.go:41` `marker.Read(opts.Target)`.
- `app/layout/layout.go:172` `isHousekeeping`: skips `.git*` and `.robbe*`. `app/plan/plan.go:23` `HousekeepingPrefix = ".robbe"` skips marker and `.robbe-tmp-*` in the target.
- `app/install/templates/robbe-sync.service`, `deploy/systemd/robbe-sync.service`: `Type=oneshot`, `ExecStart=... sync`, nothing else in `[Service]`.
- Docs naming paths: `README.md`, `docs/configuration.md`, `docs/layout.md`, `deploy/systemd/README.md`.

## Desired End State

```
                        user (euid != 0)                                root
config file   --config | $XDG_CONFIG_HOME/robbe/config.yaml             same search, ends at
                       | $XDG_CONFIG_DIRS/robbe/config.yaml (/etc/xdg)   /etc/robbe/config.yaml
                       | /etc/robbe/config.yaml
target        $XDG_CONFIG_HOME/containers/systemd/robbe                 /etc/containers/systemd/robbe
cache         $XDG_CACHE_HOME/robbe/{repo,staging}                      /var/cache/robbe/{repo,staging}
state         $XDG_STATE_HOME/robbe/applied                             /var/lib/robbe/applied
runtime       $XDG_RUNTIME_DIR/robbe/lock  (unset: <cache>/lock + WARN)  /run/robbe/lock
unit dir      $XDG_CONFIG_HOME/systemd/user                             /etc/systemd/system
```

```
internal/xdg
  ConfigHome(home) CacheHome(home) StateHome(home) RuntimeDir() ConfigDirs()
  each: env var -> absolute? use : fallback
  Paths{Config, Cache, State, Runtime, Target string}  = Defaults(user, home)
        │
        ├── config.setDefaults         (target, cache, state, runtime)
        ├── config.Init                (config search paths)
        └── install.UnitDir            (ConfigHome/systemd/user)
```

Config search resolution (`config.Init`):

```
--config given ─── yes ──▶ viper.SetConfigFile(path); ReadInConfig error is fatal
      │ no
      ▼
for dir in [ConfigHome, ConfigDirs..., /etc]: AddConfigPath(dir/robbe)
SetConfigName("config"); SetConfigType("yaml"); ReadInConfig
      not-found ─▶ fine, defaults + env
      other err ─▶ fatal (malformed yaml must not silently fall back)
```

CLI output changes:

```
$ robbe sync --dry-run
repo    git@github.com:me/infra.git ref=main
commit  abc1234 (applied: 9f8e7d6)
host    alpha  layout=hosts (common + host)
target  /home/me/.config/containers/systemd/robbe
state   /home/me/.local/state/robbe                  <- new line
...

$ XDG_RUNTIME_DIR= robbe sync            (stderr, zap WARN)
WARN  sync  XDG_RUNTIME_DIR unset, lock falls back to cache  path=/home/me/.cache/robbe/lock
```

Config file example (`~/.config/robbe/config.yaml`):

```yaml
repo:
  url: git@github.com:me/infra.git
  ref: main
host: alpha
target: /home/me/.config/containers/systemd/robbe
cache: /home/me/.cache/robbe
state: /home/me/.local/state/robbe
runtime: /run/user/1000/robbe
generator: /usr/lib/systemd/system-generators/podman-system-generator
user: true
```

Service unit:

```ini
[Service]
Type=oneshot
ExecStart={{.Binary}} sync
ConfigurationDirectory=robbe
CacheDirectory=robbe
StateDirectory=robbe
RuntimeDirectory=robbe
```

## Abstractions and Code Reuse

Existing, reused unchanged: viper key binding and `AutomaticEnv` in `config`, `marker.Read`/`marker.Write` (only the directory argument changes meaning), `Syncer.lock` flock logic, `install.Render`, `testutil.FakeRunner`, `resetViper` test helper.

- `internal/xdg/` (new)
  - `xdg.go`
    - `ConfigHome(home string) string` - `$XDG_CONFIG_HOME` if absolute, else `home/.config`.
    - `CacheHome(home string) string` - `$XDG_CACHE_HOME` or `home/.cache`.
    - `StateHome(home string) string` - `$XDG_STATE_HOME` or `home/.local/state`.
    - `RuntimeDir() (string, bool)` - `$XDG_RUNTIME_DIR` if absolute; `false` when unset. No fallback here, caller decides.
    - `ConfigDirs() []string` - `$XDG_CONFIG_DIRS` split on `:`, relative entries dropped, empty list means `["/etc/xdg"]`.
    - `absolute(value string) (string, bool)` - the shared "set and absolute" rule.
    - `Paths struct { Config, Cache, State, Runtime, Target string; RuntimeSet bool }` and `Defaults(app string, user bool, home string) Paths` - user: XDG functions joined with `app`, `Target = ConfigHome/containers/systemd/app`; root: `/etc/app`, `/var/cache/app`, `/var/lib/app`, `/run/app`, `/etc/containers/systemd/app`, `RuntimeSet = true`.
  - `xdg_test.go` - table tests per function: unset, empty, relative (ignored), absolute; `Defaults` for user and root; `ConfigDirs` with two entries and one relative.
- `config/`
  - `config.go`
    - `Config` - add `State string \`mapstructure:"state"\`` and `Runtime string \`mapstructure:"runtime"\``.
    - `ConfigFile` package var bound to `--config`.
    - `Init()` - build search paths from `xdg.ConfigHome`, `xdg.ConfigDirs`, `/etc`; `SetConfigName("config")`; honour `ConfigFile`; treat `viper.ConfigFileNotFoundError` as fine, any other read error as fatal via `cobra.CheckErr`.
    - `setDefaults(paths xdg.Paths)` - defaults for `target`, `cache`, `state`, `runtime` from `Paths`; `runtime` default is `""` when `!paths.RuntimeSet` so the fallback rule in `cmd` can see "not configured".
    - remove `defaultTarget`, `defaultCache`.
  - `config_test.go` - adjust `TestLoad_Defaults`, `TestLoad_XDG` (add state, runtime, `XDG_STATE_HOME`, `XDG_RUNTIME_DIR`), `TestLoad_YAMLFile` (write to `<home>/.config/robbe/config.yaml`), `TestLoad_EnvOverride` (add `ROBBE_STATE`, `ROBBE_RUNTIME`); new `TestInit_ConfigDirs`, `TestInit_ConfigFlag`, `TestInit_ConfigFlagMissing`, `TestInit_LegacyDotfileIgnored`, `TestInit_MalformedYAMLFatal` (via a `readConfig() error` seam so the test does not need to survive `cobra.CheckErr`).
- `cmd/`
  - `root.go` - `rootCmd.PersistentFlags().StringVar(&config.ConfigFile, "config", "", "config file (default: XDG search, see docs/configuration.md)")`.
  - `sync.go` - pass `State` and `Runtime` into `sync.Options`; `resolveRuntime(cfg, log) string` returns `cfg.Runtime` or `cfg.Cache` with WARN; `printHeader` gains the `state` line.
  - `status.go` - pass `State` into `status.Options`.
  - `install.go` - `newInstaller` no longer needs `home` except to feed `xdg.ConfigHome`; call `install.UnitDir(user, home)` unchanged in signature.
- `app/marker/marker.go` - `File = "applied"`; doc comments say "state directory"; `Read(stateDir)`, `Write(stateDir, m)`.
- `app/sync/sync.go`
  - `Options` - add `State string` (Phase 2) and `Runtime string` (Phase 3, already resolved, never empty). Every `sync.Options{}` literal (`cmd/sync.go`, `app/sync/sync_test.go`) must be updated in the same phase because `exhaustruct` is enabled.
  - `Run` - `marker.Read(s.Opts.State)`; short-circuit condition `applied.Commit == co.Commit && dirExists(s.Opts.Target)`; `marker.Write(s.Opts.State, ...)`.
  - `lock()` - `MkdirAll(s.Opts.Runtime)`, lock path `filepath.Join(s.Opts.Runtime, lockFileName)`.
  - `Result` unchanged.
- `app/status/status.go` - `Options.State`; `marker.Read(opts.State)`.
- `app/install/install.go` - `UnitDir` uses `xdg.ConfigHome(home)`.
- `app/install/templates/robbe-sync.service`, `deploy/systemd/robbe-sync.service` - four `*Directory=robbe` lines.
- `app/layout/layout.go`, `app/plan/plan.go` - comments only: the `.robbe` prefix now protects `.robbe-tmp-*`, not a marker.

## Logging & Observability

New log lines (zap, stderr):

```
WARN  sync  XDG_RUNTIME_DIR unset, lock falls back to cache  path=/home/me/.cache/robbe/lock
INFO  sync  target missing, re-applying                      commit=abc1234 target=/home/me/.config/containers/systemd/robbe
```

`robbe sync` header prints a `state` line. `--config` failures surface as cobra errors on stderr with exit 1, for example:

```
Error: read config /tmp/nope.yaml: open /tmp/nope.yaml: no such file or directory
```

## Implementation

### Phase 1: `internal/xdg` and config file relocation

Dependencies: None

Introduce the shared XDG resolver, move the config file to `$XDG_CONFIG_HOME/robbe/config.yaml` with `XDG_CONFIG_DIRS` and `/etc/robbe` fallbacks, add `--config`, and remove the duplicated XDG logic from `app/install`. `target` and `cache` defaults come from the new package; `state` and `runtime` keys are added here so the config surface changes once, but nothing consumes them until Phases 2 and 3.

**Tasks**:
- [x] `internal/xdg/xdg.go`: package doc, `absolute`, `ConfigHome`, `CacheHome`, `StateHome`, `RuntimeDir`, `ConfigDirs`, `Paths`, `Defaults`. SPDX header like the other files.
  ```go
  func absolute(value string) (string, bool) {
      if value == "" || !filepath.IsAbs(value) { return "", false }
      return filepath.Clean(value), true
  }
  func Defaults(app string, user bool, home string) Paths {
      if !user {
          return Paths{Config: "/etc/" + app, Cache: "/var/cache/" + app, State: "/var/lib/" + app,
              Runtime: "/run/" + app, Target: "/etc/containers/systemd/" + app, RuntimeSet: true}
      }
      rt, ok := RuntimeDir()
      // ...
  }
  ```
- [x] `internal/xdg/xdg_test.go`: table tests for every function covering unset, empty, relative, absolute values (`t.Setenv`); `TestConfigDirs` with `a:/b::c/d` yielding `[/b]` and unset yielding `[/etc/xdg]`; `TestDefaults_User`, `TestDefaults_Root`, `TestDefaults_RuntimeUnset`.
- [x] `config/config.go`: add `State`, `Runtime` to `Config`; add `var ConfigFile string`; rewrite `Init` to use `xdg.Defaults` and the search order; `SetConfigName("config")`; split reading into `readConfig() error` returning `nil` on `viper.ConfigFileNotFoundError` (only when `ConfigFile == ""`) and wrapping other errors as `read config %s: %w`; `Init` calls `cobra.CheckErr(readConfig())`. Delete `defaultTarget`, `defaultCache`; `setDefaults(paths xdg.Paths)` registers `target`, `cache`, `state`, `runtime` (`runtime` = `""` when `!paths.RuntimeSet`). Remove the `runtime` import.
- [x] `cmd/root.go`: register the persistent `--config` flag bound to `config.ConfigFile`.
- [x] `app/install/install.go`: `UnitDir` calls `xdg.ConfigHome(home)`; drop the inline env lookup.
- [x] `config/config_test.go`: update `resetViper` to also clear `XDG_CONFIG_HOME`, `XDG_CONFIG_DIRS`, `XDG_STATE_HOME`, `XDG_RUNTIME_DIR` and `ConfigFile`; `TestLoad_Defaults` expects `State = <home>/.local/state/robbe`, `Runtime = ""` when `XDG_RUNTIME_DIR` is empty; `TestLoad_XDG` sets all four vars and checks `State`, `Runtime`; `TestLoad_EnvOverride` adds `ROBBE_STATE=/s`, `ROBBE_RUNTIME=/r`; `TestLoad_YAMLFile` writes `<home>/.config/robbe/config.yaml`; new `TestInit_ConfigDirs` (file only under a temp `XDG_CONFIG_DIRS` entry is found), `TestInit_LegacyDotfileIgnored` (`~/.robbe.yaml` and `./.robbe.yaml` present, neither read), `TestInit_ConfigFlag` (explicit path outside the search wins), `TestInit_ConfigFlagMissing` (`readConfig` returns an error), `TestInit_MalformedYAML` (`readConfig` returns an error for `key: [` in the search path).
- [x] `app/install/install_test.go` `TestUnitDir`: add a relative `XDG_CONFIG_HOME` case that falls back to `<home>/.config/systemd/user`.
- [x] `docs/configuration.md`: rewrite "Config file search order" (flag, `$XDG_CONFIG_HOME/robbe/config.yaml`, `$XDG_CONFIG_DIRS`, `/etc/robbe/config.yaml`; malformed file is an error; legacy paths not read), add `state` and `runtime` rows to the keys table with defaults and the runtime fallback rule, add an "XDG variables" paragraph (empty or relative values ignored), update the examples to `~/.config/robbe/config.yaml` and include `state`/`runtime`.
- [x] `README.md`: quick start writes `~/.config/robbe/config.yaml` (`install -d ~/.config/robbe` first); mention `--config`.
- [x] `deploy/systemd/README.md`: rootless step writes `~/.config/robbe/config.yaml`; root step `install -Dm600 /dev/null /etc/robbe/config.yaml`.

**Automated Verification**:
- [x] `go test ./internal/xdg/...` passes.
- [x] `go test ./config/... ./app/install/...` passes, including `TestInit_ConfigDirs`, `TestInit_LegacyDotfileIgnored`, `TestInit_ConfigFlag`, `TestInit_ConfigFlagMissing`, `TestInit_MalformedYAML`.
- [x] `HOME=$(mktemp -d) XDG_CONFIG_HOME=$(mktemp -d) sh -c 'mkdir -p $XDG_CONFIG_HOME/robbe && printf "repo:\n  url: https://example.com/x.git\n" > $XDG_CONFIG_HOME/robbe/config.yaml && go run . status' ` reaches the fetch step (fails on the unreachable URL, not on `repo.url is not configured`).
- [x] `go run . --config /nonexistent.yaml status` exits 1 with `read config /nonexistent.yaml` on stderr.
- [x] `grep -rn '\.robbe\.yaml' --include=*.go --include=*.md . | grep -v docs/agents` prints nothing.
- [x] `golangci-lint run ./...` passes.

### Phase 2: Marker in the state directory

Dependencies: Phase 1

Move the applied-commit marker from the target to `<state>/applied`, guard the short-circuit against a wiped target, wire `state` through sync and status.

**Tasks**:
- [x] `app/marker/marker.go`: `File = "applied"`; package and function docs refer to the state directory; parameter renamed `stateDir`. Behaviour unchanged.
- [x] `app/marker/marker_test.go`: rename fixture paths, add a case where the state directory does not exist yet and `Write` creates it.
- [x] `app/sync/sync.go`: `Options.State string`; `Run` reads and writes the marker from `s.Opts.State`; short-circuit requires the target directory to exist, otherwise log `target missing, re-applying` and continue; add `dirExists(path string) (bool, error)` helper (or reuse the pattern from `layout.isDir`).
- [x] `app/sync/sync_test.go`: harness gets a `state` temp dir passed as `Options.State`; `TestSync_Apply` asserts the marker at `<state>/applied` and asserts no `.robbe-commit` in the target; new `TestSync_TargetWipedReapplies` (apply, `os.RemoveAll(target)`, run again, `Applied == true`, files back); `TestSync_UnitFailureKeepsMarker` reads from the state dir.
- [x] `app/status/status.go`: `Options.State`; `marker.Read(opts.State)`.
- [x] `app/status/status_test.go`: write the marker to the state dir, assert `Applied`.
- [x] `cmd/sync.go`: pass `State: cfg.State` into `sync.Options`; `printHeader` prints `state   <path>` after `target`.
- [x] `cmd/status.go`: pass `State: cfg.State`.
- [x] `app/layout/layout.go`, `app/plan/plan.go`: update the `isHousekeeping` / `HousekeepingPrefix` comments (`.robbe-tmp-*` only, marker no longer lives here); no logic change. `app/layout/layout_test.go:40` fixture entry `.robbe-commit` may stay as a "hidden housekeeping file in a repo is skipped" case.
- [x] `docs/layout.md`: intro no longer mentions the marker in the target; "Ignored files" explains `.robbe-*` as robbe's temporary files.
- [x] `docs/configuration.md`: `state` row describes the `applied` file and the wiped-target rule.
- [x] `README.md`: replace `<target>/.robbe-commit` with `<state>/applied` in "Commands" and "How a sync works"; add the wiped-target sentence to step 2.

**Automated Verification**:
- [x] `go test ./app/marker/... ./app/sync/... ./app/status/... ./cmd/...` passes, including `TestSync_TargetWipedReapplies`.
- [x] `grep -rn 'robbe-commit' --include=*.go --include=*.md . | grep -v docs/agents` prints nothing.
- [x] `golangci-lint run ./...` passes.

**Manual Verification**:
- [ ] On a rootless host with a configured repo: `robbe sync`, then `ls -a ~/.config/containers/systemd/robbe` shows no dotfiles and `cat ~/.local/state/robbe/applied` shows the commit and timestamp.
- [ ] `rm -rf ~/.config/containers/systemd/robbe && robbe sync` re-applies instead of printing `up to date`.

### Phase 3: Lock in the runtime directory

Dependencies: Phase 1

Move the flock file to `<runtime>/lock`, resolve the fallback in `cmd` with a warning.

**Tasks**:
- [x] `app/sync/sync.go`: `Options.Runtime string` (documented as already resolved, never empty); `lock()` creates `s.Opts.Runtime` and opens `<runtime>/lock`; drop `MkdirAll(Cache)` from `lock()` only if `gogit`/staging still create the cache themselves (they do: `gogit.open` and `stageAndValidate`), otherwise keep it.
- [x] `app/sync/sync_test.go`: harness passes a `runtime` temp dir; `TestSync_Locked` locks `<runtime>/lock`; new `TestSync_LockDirCreated` asserts the lock file appears under a not-yet-existing runtime dir.
- [x] `cmd/sync.go`: `resolveRuntime(cfg config.Config, log *zap.Logger) string` returns `cfg.Runtime` when non-empty, else `cfg.Cache` after `log.Warn("XDG_RUNTIME_DIR unset, lock falls back to cache", zap.String("path", ...))`; pass the result as `Options.Runtime`.
- [x] `cmd/sync_test.go` (new, package `cmd`): `TestResolveRuntime` with a `zaptest/observer` core asserting the warning fires only for the empty case.
- [x] `docs/configuration.md`: `runtime` row documents the lock file, `/run/robbe` for root, and the cache fallback with warning.
- [x] `README.md`: one sentence under "How a sync works" about the lock location (concurrent runs exit with `another robbe run is in progress`).

**Automated Verification**:
- [x] `go test ./app/sync/... ./cmd/...` passes, including `TestSync_LockDirCreated` and `TestResolveRuntime`.
- [x] `XDG_RUNTIME_DIR= ROBBE_REPO_URL=https://example.com/x.git go run . sync --dry-run 2>&1 | grep -q 'lock falls back to cache'` succeeds.
- [x] `golangci-lint run ./...` passes.

### Phase 4: systemd directory directives

Dependencies: Phases 2 and 3 (paths must be final before the units declare them)

Let systemd create the four directories for timer-driven runs.

**Tasks**:
- [x] `app/install/templates/robbe-sync.service`: append `ConfigurationDirectory=robbe`, `CacheDirectory=robbe`, `StateDirectory=robbe`, `RuntimeDirectory=robbe` to `[Service]`.
- [x] `deploy/systemd/robbe-sync.service`: same four lines.
- [x] `app/install/install_test.go` `TestRender`: assert all four directives are present in the rendered service.
- [x] `hack/` or Makefile check: add a `make check-units` target (or a step in an existing target) running `diff <(sed 's#/usr/local/bin/robbe#{{.Binary}}#' deploy/systemd/robbe-sync.service) app/install/templates/robbe-sync.service` so the reference copy and the template cannot drift; wire it into `make lint`.
- [x] `deploy/systemd/README.md`: paragraph explaining that the directives create `/etc/robbe`, `/var/cache/robbe`, `/var/lib/robbe`, `/run/robbe` (root) or the XDG equivalents (user), and that manual `robbe sync` runs create the same directories themselves; fix the stale "default `5m`" interval mention to `10s` (the code default changed in commit `1cbf7d9`).

**Automated Verification**:
- [x] `go test ./app/install/...` passes.
- [x] `make check-units` passes (template and reference unit agree).
- [x] `systemd-analyze verify deploy/systemd/robbe-sync.service` reports no errors for the `[Service]` section (ExecStart path may be reported missing on a dev machine; only directive errors count).
- [x] `golangci-lint run ./...` and `make test` pass.

**Manual Verification**:
- [ ] `robbe install` on a rootless host, wait one interval: `ls $XDG_RUNTIME_DIR/robbe` exists while the service runs, `~/.local/state/robbe/applied` is written, `journalctl --user -u robbe-sync.service` shows a normal sync run without the runtime-dir warning.

## Implementation Notes

During implementation, document user feedback, problems, and decisions here.

- Phase 1: `setDefaults` takes `(paths xdg.Paths, user bool)` because the `user` default still comes from `os.Geteuid()`, not from `Paths`. `Init` is split into `Init()` (calls `cobra.CheckErr`) and `configure() error`, which the `TestInit_ConfigFlagMissing` / `TestInit_MalformedYAML` tests call directly instead of a bare `readConfig` seam, so the search path setup is exercised too.
- Phase 1: the `grep '\.robbe\.yaml'` check still hits two intentional places: the sentence in `docs/configuration.md` stating the legacy locations are not read, and `TestInit_LegacyDotfileIgnored`, which writes those files to prove they are ignored. No code path reads them.
- Phase 2: `TestDiff_MarkerIgnored` in `app/plan/plan_test.go` was removed: it asserted the marker inside the target is skipped, which no longer describes a real situation. `TestDiff_StaleTempIgnored` (`.robbe-tmp-*`) still covers the housekeeping prefix. The `layout_test.go` fixture entry `.robbe-commit` was renamed to `.robbe-tmp-1` for the same reason and so the `robbe-commit` grep is clean.
- Phase 2: `Run` was over the `cyclop`/`funlen` limits after the target-exists guard; the short-circuit decision moved into `Syncer.upToDate`.
- Phase 3: `lock()` no longer creates the cache directory; `gogit.open` and `stageAndValidate` create it themselves, as the plan anticipated. `TestSync_LockDirCreated` also asserts no lock appears in the cache.
- Phase 4: `make check-units` pipes `sed` into `diff -` instead of using bash process substitution, so it also works when `make` runs under dash. `make lint` depends on it. `systemd-analyze verify` reports only the missing `/usr/local/bin/robbe` binary on the dev machine, no directive errors.
- Phase 1: `paralleltest` does not recognise `t.Setenv` inside a table loop; the affected `internal/xdg` and `config` tests carry `//nolint:paralleltest` with a reason, matching the existing `TestLoad_MissingURL` precedent.

## References

- XDG Base Directory Specification 0.8: https://specifications.freedesktop.org/basedir-spec/latest/ (`XDG_CONFIG_HOME`, `XDG_CONFIG_DIRS` default `/etc/xdg`, `XDG_STATE_HOME` default `~/.local/state`, `XDG_RUNTIME_DIR` without default, "relative paths must be ignored").
- systemd.exec(5): `RuntimeDirectory=`, `StateDirectory=`, `CacheDirectory=`, `ConfigurationDirectory=` and their user-instance mapping onto `$XDG_RUNTIME_DIR`, `$XDG_STATE_HOME`, `$XDG_CACHE_HOME`, `$XDG_CONFIG_HOME`.
- viper: `SetConfigFile`, `AddConfigPath`, `ConfigFileNotFoundError`.
- Previous plan: `docs/agents/plans/2026-09-20-quadlet-gitops.md` (decision 4 placed the marker in the target; superseded by decision 2 here).
