---
date: 2026-09-22T05:15:16Z
git_commit: 0dc5289607ba8e1dcd2b9ceca96fd00c5ff231f6
branch: main
topic: "Colored sync plan output"
tags: [plan, cmd, sync, plan, ansi, cli]
status: ready
---

# PLAN: Colored sync plan output

Print the files/units plan of `robbe sync` and `robbe sync --dry-run` in color when stdout is a terminal that supports it. Piped output, systemd journal output, `NO_COLOR` and `TERM=dumb` keep today's plain text byte for byte. A persistent `--color auto|always|never` flag overrides detection.

## Acceptance Criteria

- `robbe sync --dry-run` and `robbe sync` on a TTY print the plan colored: `+` lines and `start` lines green, `~` lines and `restart` lines yellow, `-` lines and `stop` lines red, the `files` and `units` section headers bold, `(none)` and the `(support file -> restart workloads)` note dim. Every colored line ends with a reset.
- The header lines (`repo`, `commit`, `host`, `target`, `state`) and the trailer lines (`dry run: no changes made`, `applied <hash>`, `up to date`) stay plain.
- When stdout is not a TTY, or `NO_COLOR` is set to any value (including empty), or `TERM` is `dumb`, the output is byte-identical to today's output.
- `--color=never` always prints plain; `--color=always` always prints colored, even when piped or with `NO_COLOR` set.
- Any other `--color` value fails with an error mentioning the accepted values, exit status 1, before config is loaded or the repository is fetched.
- `plan.Plan.String()` output is unchanged; the existing tests in `app/plan/plan_test.go` pass without modification.
- `go build ./...`, `make test` and `golangci-lint run ./...` pass.
- README and `docs/configuration.md` mention `--color` and `NO_COLOR`.

## Technical Key Decisions and Tradeoffs

1. **Persistent root flag `--color auto|always|never`, default `auto`:** declared on `rootCmd` in `cmd/root.go`.
   - Why: one flag for every command so `status` can reuse it later; the systemd timer run stays plain through detection, not through configuration.
   - Impact: only `sync` reads the flag in this plan. The value is validated in a `PersistentPreRunE` on the root command so a typo fails before any work is done.
2. **`Plan.Render(w io.Writer, s Style)` in `app/plan`; `String()` renders with the zero `Style`:** `Style` is a struct of prefix strings plus a `Reset` suffix.
   - Why: the application layer keeps the layout rules; the terminal knowledge (escape codes, TTY detection) stays in `cmd` and `internal/ansi`. The zero value produces exactly today's text, so nothing in `app/sync` or the tests changes.
   - Impact: `writeFiles` and the unit loops take the style; a line is wrapped as `prefix + text + reset` only when the prefix is non-empty.
3. **`golang.org/x/term` v0.46.0 for `IsTerminal`; ANSI codes hand-rolled:** one new direct dependency, pairs with the `golang.org/x/sys` v0.48.0 already in the module graph.
   - Why: the canonical Go-team terminal library, tiny; a color framework (`fatih/color`, `termenv`) would bring its own auto-detection that fights the `--color` flag.
   - Impact: `go get golang.org/x/term@v0.46.0`, six escape constants in `internal/ansi`.
4. **`internal/ansi` holds the constants and the enable rule:** `Enabled(mode string, stdout *os.File) (bool, error)`.
   - Why: the rule (flag, `NO_COLOR`, `TERM=dumb`, TTY) is unit-testable without cobra, and `internal/` cannot import `app/plan`, so the mapping onto `plan.Style` lives in `cmd`.
   - Impact: `cmd/sync.go` calls `ansi.Enabled(colorMode, os.Stdout)` once and builds the style.
5. **`--color=always` wins over `NO_COLOR` and a non-TTY:** per no-color.org, an explicit user flag overrides the environment.
6. **Color decision made from `os.Stdout`, not from `cmd.OutOrStdout()`:** tests that set a buffer on the command see plain output unless they pass `--color=always`.
7. **Documentation:** the user explicitly authorized one sentence in `README.md` under "Commands" and a `NO_COLOR` paragraph in `docs/configuration.md` next to the `DEVELOPMENT` paragraph.

## Current State

```
cmd/sync.go
  RunE ─┬─ syncer.Run(ctx, dryRun)            -> sync.Result{Plan plan.Plan, ...}
        ├─ printHeader(out, cfg, res)         plain fmt.Fprintf
        └─ fmt.Fprint(out, res.Plan.String()) plain, out = cmd.OutOrStdout()

app/plan/plan.go
  Plan.String()   strings.Builder: "files\n", writeFiles("+"/"~"/"-"),
                  "\nunits\n", stop/start/restart lines, "(none)" fallbacks
  writeFiles()    "  %s %s%s\n" with the support-file note

app/plan/plan_test.go:407  TestDiff_String asserts substrings of String()
cmd/root.go                rootCmd, persistent --config flag only
```

No TTY detection, no color, no dependency on `golang.org/x/term` (only a stale go.sum line). Output of `robbe sync --dry-run` today:

```
repo    git@github.com:me/infra.git ref=main
commit  abc1234 (applied: 9f8e7d6)
host    alpha  layout=hosts (common + host)
target  /home/me/.config/containers/systemd/robbe
state   /home/me/.local/state/robbe

files
  + hosts/alpha/nginx.container
  ~ common/proxy.network
  - db.container
  ~ nginx.env          (support file -> restart workloads)

units
  stop     db.service
  start    nginx.service
  restart  proxy-network.service
dry run: no changes made
```

## Desired End State

```
cmd/root.go      --color auto|always|never (persistent, validated in PersistentPreRunE)
      │
cmd/sync.go      enabled, _ := ansi.Enabled(colorMode, os.Stdout)
      │          style := planStyle(enabled)          // plan.Style{} or ANSI-filled
      │          res.Plan.Render(out, style)
      ▼
app/plan         Plan.Render(w, Style)   layout as today, each line wrapped in style
                 Plan.String()           = Render(zero Style)
      ▲
internal/ansi    Bold, Dim, Red, Green, Yellow, Reset constants
                 Enabled(mode, f) : always -> true | never -> false |
                                    auto -> !NO_COLOR && TERM!="dumb" && term.IsTerminal(f)
```

Output on a terminal (escape codes shown as `<name>`):

```
repo    git@github.com:me/infra.git ref=main
commit  abc1234 (applied: 9f8e7d6)
host    alpha  layout=hosts (common + host)
target  /home/me/.config/containers/systemd/robbe
state   /home/me/.local/state/robbe

<bold>files<reset>
<green>  + hosts/alpha/nginx.container<reset>
<yellow>  ~ common/proxy.network<reset>
<red>  - db.container<reset>
<yellow>  ~ nginx.env<reset><dim>          (support file -> restart workloads)<reset>

<bold>units<reset>
<red>  stop     db.service<reset>
<green>  start    nginx.service<reset>
<yellow>  restart  proxy-network.service<reset>
dry run: no changes made
```

Empty plan on a terminal:

```
<bold>files<reset>
<dim>  (none)<reset>

<bold>units<reset>
<dim>  (none)<reset>
```

Piped (`robbe sync --dry-run | cat`), under the systemd timer, with `NO_COLOR=1`, with `TERM=dumb`, or with `--color=never`: identical to the current output above.

## Abstractions and Code Reuse

Reused: `plan.Plan` field layout, `cmd.OutOrStdout()` for the writer, the cobra persistent-flag pattern already used for `--config`, the table-driven test style of `cmd/sync_test.go`.

New:

- `internal/ansi/`
  - `ansi.go` - package doc, SGR constants, `Enabled`.
    - `Bold`, `Dim`, `Red`, `Green`, `Yellow`, `Reset` - `"\x1b[1m"`, `"\x1b[2m"`, `"\x1b[31m"`, `"\x1b[32m"`, `"\x1b[33m"`, `"\x1b[0m"`.
    - `ModeAuto`, `ModeAlways`, `ModeNever` - `"auto"`, `"always"`, `"never"`.
    - `ErrInvalidMode` - sentinel wrapped by `Enabled` and `ValidMode`.
    - `ValidMode(mode string) error` - nil for the three modes, `ErrInvalidMode` otherwise; used by the root flag validation.
    - `Enabled(mode string, f *os.File) (bool, error)` - the rule from decision 4; `f` may be nil (treated as not a terminal) so tests can cover the non-TTY branch without a pipe.
  - `ansi_test.go` - table over mode, `NO_COLOR`, `TERM`, TTY-ness. Uses `t.Setenv`, so no `t.Parallel()` in this test.
- `app/plan/`
  - `plan.go`
    - `Style` - `struct{ Header, Add, Change, Remove, Start, Restart, Stop, Muted, Reset string }`.
    - `Plan.Render(w io.Writer, s Style)` - the current body of `String()`, writing through `s`.
    - `Plan.String()` - `var b strings.Builder; p.Render(&b, Style{}); return b.String()`.
    - `writeFiles(w io.Writer, s Style, sign, prefix string, files []File)` - one `span` for `"  " + sign + " " + rel` in `prefix`, then for a support file a second `span` for the note in `s.Muted`, then `"\n"`. Both spans share the line, matching the mockup.
    - `span(w io.Writer, prefix, text, reset string)` - helper writing `prefix+text+reset` without a newline; when `prefix == ""` writes only `text` so the zero style emits no stray resets. Every rendered line is one or two spans followed by `"\n"`.
  - `plan_test.go`
    - `TestPlan_Render_Styled` - new test with a fully populated `Style` using short marker strings (`"<a>"`, `"</>"`) and exact-string comparison of the whole output; `TestDiff_String` untouched.
- `cmd/`
  - `root.go`
    - `colorMode string` - bound to persistent `--color`, default `ansi.ModeAuto`.
    - `rootCmd.PersistentPreRunE` - `ansi.ValidMode(colorMode)`, wrapped as `--color: %w`.
  - `sync.go`
    - `RunE` - replaces both `fmt.Fprint(out, res.Plan.String())` calls with `res.Plan.Render(out, style)` where `style` comes from `planStyle(colorEnabled())`.
    - `colorEnabled() bool` - `ansi.Enabled(colorMode, os.Stdout)`; an error cannot occur after `PersistentPreRunE` validated the mode, but is still logged at warn level and treated as disabled.
    - `planStyle(enabled bool) plan.Style` - zero style, or `{Header: ansi.Bold, Add: ansi.Green, Change: ansi.Yellow, Remove: ansi.Red, Start: ansi.Green, Restart: ansi.Yellow, Stop: ansi.Red, Muted: ansi.Dim, Reset: ansi.Reset}`.
  - `sync_test.go`
    - `TestPlanStyle` - disabled gives the zero style; enabled fills every field.
    - `TestRootColorFlag_Invalid` - executing `rootCmd` with `--color=blue version` returns an error containing `--color` and `auto`.
- `go.mod` / `go.sum` - `golang.org/x/term v0.46.0` as a direct requirement.
- `README.md` - one sentence under "Commands" (authorized by the user).
- `docs/configuration.md` - `NO_COLOR` paragraph after the `DEVELOPMENT` paragraph (authorized by the user).

## Logging & Observability

No new log lines on the normal path. One defensive warning:

```
WARN  sync  color detection failed, printing plain   err="..."
```

emitted only if `ansi.Enabled` returns an error, which the flag validation makes unreachable in practice.

## Implementation

### Phase 1: Styled rendering core

Dependencies: None

Introduce `plan.Style` and `Plan.Render`, the `internal/ansi` package with the enable rule, and the `x/term` dependency. Nothing user-visible changes yet; `String()` stays byte-identical.

**Tasks**:

- [x] `go get golang.org/x/term@v0.46.0 && go mod tidy`; confirm `golang.org/x/term v0.46.0` appears in the direct `require` block of `go.mod`.
- [x] `internal/ansi/ansi.go`: package doc (`// Package ansi holds the SGR escape sequences robbe prints and the rule for when to print them.`), SPDX header matching the other files, the six SGR constants, the three mode constants, `ErrInvalidMode`, `ValidMode`, `Enabled`.

  ```go
  func Enabled(mode string, f *os.File) (bool, error) {
      switch mode {
      case ModeAlways:
          return true, nil
      case ModeNever:
          return false, nil
      case ModeAuto:
          if _, set := os.LookupEnv("NO_COLOR"); set {
              return false, nil
          }
          if os.Getenv("TERM") == "dumb" {
              return false, nil
          }
          return f != nil && term.IsTerminal(int(f.Fd())), nil
      default:
          return false, fmt.Errorf("%w: %q (want auto, always or never)", ErrInvalidMode, mode)
      }
  }
  ```

- [x] `internal/ansi/ansi_test.go`: `TestEnabled` table: `always` with `NO_COLOR=1` and nil file gives true; `never` on any input gives false; `auto` with `NO_COLOR=""` (set but empty) gives false; `auto` with `TERM=dumb` gives false; `auto` with a temp file (`os.CreateTemp`) gives false; `auto` with nil file gives false; `auto` with `/dev/tty` opened read-only gives true when the open succeeds, `t.Skip` when it fails (CI has no controlling terminal); `"blue"` gives `ErrInvalidMode` via `errors.Is`. `TestValidMode` covers the three modes and one invalid value.
- [x] `app/plan/plan.go`: add `Style` with a doc comment stating that the zero value renders plain text and that every non-empty prefix is closed with `Reset`; add `Render`; rewrite `String()` as a wrapper; change `writeFiles` to take `io.Writer`, the style prefix and `Style`; add `span`. The unit loops use `s.Stop`, `s.Start`, `s.Restart`; the `(none)` lines use `s.Muted`; `files` and `units` use `s.Header`. Keep the exact spacing of every line.
- [x] `app/plan/plan_test.go`: add `TestPlan_Render_Styled` building a `Plan` literal with one entry per list including a support file, rendering with `Style{Header: "<h>", Add: "<a>", Change: "<c>", Remove: "<r>", Start: "<s>", Restart: "<rs>", Stop: "<st>", Muted: "<m>", Reset: "</>"}` and comparing against the exact expected string. Add `TestPlan_Render_ZeroStyleEqualsString` asserting `Render(&b, Style{})` equals `String()` for the same populated plan and for the zero plan, and that the output contains no `\x1b`.

**Automated Verification**:

- [x] `go test -race -shuffle=on ./internal/ansi/ ./app/plan/` passes, including the untouched `TestDiff_String`.
- [x] `go build ./...` passes.
- [x] `golangci-lint run ./...` passes.
- [x] `go list -m golang.org/x/term` prints `golang.org/x/term v0.46.0`.

### Phase 2: CLI wiring and documentation

Dependencies: Phase 1

Add the `--color` flag, wire detection into `sync`, and document the surface.

**Tasks**:

- [x] `cmd/root.go`: `var colorMode string`; in `init()` add `rootCmd.PersistentFlags().StringVar(&colorMode, "color", ansi.ModeAuto, "colorize output: auto, always or never")`; add `PersistentPreRunE: func(*cobra.Command, []string) error { if err := ansi.ValidMode(colorMode); err != nil { return fmt.Errorf("--color: %w", err) }; return nil }`. `SilenceUsage` stays true so the error is a single line.
- [x] `cmd/sync.go`: add `colorEnabled` and `planStyle`; in `RunE` compute `style := planStyle(colorEnabled(log))` once before the switch and replace both `fmt.Fprint(out, res.Plan.String())` with `res.Plan.Render(out, style)`. Import `os` and `internal/ansi`.
- [x] `cmd/sync_test.go`: add `TestPlanStyle` (zero when disabled, every field non-empty and `Reset == ansi.Reset` when enabled) and `TestRootColorFlag_Invalid` (`rootCmd.SetArgs([]string{"--color", "blue", "version"})`, `SetOut`/`SetErr` to buffers, `Execute()` returns an error whose message contains `--color` and `auto`). Reset `rootCmd` args and `colorMode` with `t.Cleanup`.
- [x] `README.md` "Commands" section, after the exit-status paragraph: `On a terminal the files/units plan is printed in color; \`--color=never\` or \`--color=always\` overrides the detection and \`NO_COLOR\` is respected.`
- [x] `docs/configuration.md` "Environment variables" section, after the `DEVELOPMENT` paragraph: a paragraph stating that `NO_COLOR` (set to any value, per <https://no-color.org>) disables colored output of `robbe sync`, that `TERM=dumb` and a non-terminal stdout do the same, that the persistent `--color=always|never` flag overrides both, and that neither is a `ROBBE_*` key nor part of `Config`.

**Automated Verification**:

- [x] `make test` passes.
- [x] `golangci-lint run ./...` passes.
- [x] `go build -o bin/robbe . && ROBBE_REPO_URL=<local repo from internal/testutil, or any reachable repo> bin/robbe sync --dry-run --color=never | grep -c $'\x1b'` prints `0`.
- [x] Same command with `--color=always` piped through `grep -c $'\x1b'` prints a number greater than `0`.
- [x] `bin/robbe --color=blue version` exits 1 and prints one line containing `--color` and `auto`.
- [x] `make check-units` still passes (no unit files touched, guard against accidental edits).

**Manual Verification**:

- [ ] In an interactive terminal, `bin/robbe sync --dry-run` against a repository with at least one add, one change, one remove and one support file shows green `+`/`start`, yellow `~`/`restart`, red `-`/`stop`, bold `files`/`units`, dim note.
- [ ] `bin/robbe sync --dry-run | cat` in the same terminal shows plain text.
- [ ] `NO_COLOR=1 bin/robbe sync --dry-run` shows plain text; `NO_COLOR=1 bin/robbe sync --dry-run --color=always` shows color.

## Implementation Notes

During implementation, document user feedback, problems, and decisions here.

- 2026-09-22: `io.WriteString` in `Render` tripped errcheck/gosec (G104); switched to `fmt.Fprint`/`fmt.Fprintln`, which the `std-error-handling` exclusion preset covers. Added a `line` helper (span + newline) beside `span` to keep `Render` flat.
- 2026-09-22: exhaustruct_v5 rejects `Style{}` as a call argument (only empty returns and declarations are allowed), so `String()` and the tests use `var s Style` instead of the literal from the plan.
- 2026-09-22: `TestEnabled` restores the caller's `NO_COLOR` via `t.Setenv` before unsetting it, so a developer shell with `NO_COLOR` exported does not break the `auto with tty` case. `TestRootColorFlag_Invalid` carries `//nolint:paralleltest` like the viper tests in `config/`.
- 2026-09-22: Phase 2 automated checks ran against a scratch repo (one `.container`, one `.env`) with `ROBBE_REPO_URL`/`ROBBE_TARGET`/`ROBBE_CACHE`/`ROBBE_STATE` pointed at the scratchpad: `--color=never` and piped auto print 0 escapes, `--color=always` prints 5, `NO_COLOR=1 --color=always` still prints 5, `--color=blue version` exits 1 with one `Error: --color: invalid color mode: "blue" (want auto, always or never)` line.

## References

- `cmd/sync.go:63-83` current plan printing.
- `app/plan/plan.go:156-199` `Plan.String()` and `writeFiles`.
- `app/plan/plan_test.go:407-428` existing `String()` assertions that must keep passing.
- `cmd/root.go:31` persistent flag pattern.
- no-color.org: <https://no-color.org/>
- `golang.org/x/term`: <https://pkg.go.dev/golang.org/x/term#IsTerminal>
- Prior plan for output layout: `docs/agents/plans/2026-09-20-quadlet-gitops.md` (CLI mockups section).
