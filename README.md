# robbe

robbe is a small Go CLI that keeps a host's Podman Quadlet units in sync with a
git repository. A systemd timer runs `robbe sync` periodically; each run
fetches the repository, computes the desired set of quadlet files for this
host, validates them with the quadlet generator, writes them into a directory
owned by robbe and starts, restarts or stops the affected systemd units. No
daemon, no Kubernetes.

## Quick start

```sh
# build
make build                      # -> bin/robbe

# configure (see docs/configuration.md for every key)
cat > ~/.robbe.yaml <<YAML
repo:
  url: git@github.com:me/infra.git
  ref: main
YAML

# try it
bin/robbe sync --dry-run        # fetch and print the plan, change nothing
bin/robbe sync                  # apply
bin/robbe status                # applied commit, remote head, drift

# run every 5 minutes
install -Dm755 bin/robbe ~/.local/bin/robbe
~/.local/bin/robbe install --interval 5m
loginctl enable-linger "$USER"  # rootless only: keep the timer running without a login
```

Quadlet files land in `~/.config/containers/systemd/robbe/` (rootless) or
`/etc/containers/systemd/robbe/` (root). Nothing outside that directory is
touched.

## Commands

| command                     | what it does                                                                                           |
|-----------------------------|--------------------------------------------------------------------------------------------------------|
| `robbe sync`                | Fetch, resolve the host's tree, validate with `podman-system-generator --dryrun`, write files, drive systemd. |
| `robbe sync --dry-run`      | Same, but stop after printing the plan.                                                                |
| `robbe sync --allow-empty`  | Permit a run whose desired tree is empty (removes every managed unit).                                 |
| `robbe status`              | Applied commit, remote head of `ref`, drift between target and repository.                             |
| `robbe install [--interval 5m]` | Write `robbe-sync.service` and `robbe-sync.timer`, `daemon-reload`, `enable --now` the timer.      |
| `robbe uninstall`           | Disable the timer and remove both units.                                                               |
| `robbe version`             | Print the build version.                                                                               |

Exit status is 0 on success or when nothing changed, 1 on any error.
`sync` skips all work when the fetched commit equals the one recorded in
`<target>/.robbe-commit`.

## How a sync works

1. Clone or fetch the repository into the cache (`go-git`, no `git` binary needed) and check out `ref`.
2. Compare the commit with `<target>/.robbe-commit`; exit if equal.
3. Resolve the desired tree: `common/` overlaid by `hosts/<hostname>/`, or the repository root (see [docs/layout.md](docs/layout.md)).
4. Stage the tree and run `podman-system-generator --dryrun` over it. Invalid quadlets abort the run before the target is touched.
5. Diff staging against the target: files to add, change, remove; units to start, restart, stop.
6. Stop removed units, delete their files, write added and changed files, `daemon-reload`, restart changed and start added units in one systemd transaction, verify with `is-active`.
7. Record the commit in `.robbe-commit` when every unit action succeeded.

## Documentation

- [docs/configuration.md](docs/configuration.md): config file, environment variables, authentication.
- [docs/layout.md](docs/layout.md): repository layouts, merge rule, restart rule.
- [deploy/systemd](deploy/systemd): reference units and manual installation.

## Development

```sh
make test               # unit tests
make test-integration   # also runs the real generator when installed
make lint               # golangci-lint
```

## Project layout

```
.
├── .github/    GitHub configuration: composite actions (.github/actions),
│               the ci and pr-title workflows, and the branch ruleset that
│               declares which checks are required to merge.
│
├── adapters/   Hexagonal architecture: adapters (go-git, generator,
├── app/        systemctl, os/exec), app (layout, plan, sync, status,
├── ports/      install) and the ports they meet at.
├── quadlet/    Quadlet naming rules shared by app and adapters.
│
├── cmd/        Cobra command implementations. Holds the root command and
│               its subcommands; main.go only calls cmd.Execute().
├── config/     Viper-based configuration loading and the application's
│               default values
├── telemetry/  Telemetry package, holds logger.
│
├── deploy/     Deployment files (systemd service and timer units)
├── hack/       Developer and CI tooling that is not part of the shipped
│               binary. hooks/ holds the Conventional Commits validator
│               shared by the local commit-msg hook and the pr-title
│               workflow.
└── docs/       Documentation for humans.
```
