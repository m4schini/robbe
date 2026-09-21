# Repository layout

robbe reads quadlet files from a git repository and writes the set that
applies to the current host into `<quadlet dir>/robbe/`
(`~/.config/containers/systemd/robbe/` for a user, `/etc/containers/systemd/robbe/`
for root). Quadlet searches that subdirectory like any other, so unit names
are the same as if the files were placed in the quadlet directory directly.

## Layouts

Two layouts are accepted. Which one applies is decided by the presence of a
`hosts/` directory at the repository root.

### Single host (flat)

Without `hosts/`, the repository root is the desired tree:

```
repo/
├── nginx.container
├── nginx.env
└── web.network
```

Every file (except git metadata, robbe housekeeping files and `.kube` files,
see below) is written to the target directory with the same relative path.

### Multiple hosts

With `hosts/`, the desired tree is `common/` overlaid by `hosts/<hostname>/`:

```
repo/
├── common/
│   └── proxy.network
└── hosts/
    ├── alpha/
    │   ├── nginx.container
    │   └── nginx.env
    └── beta/
        └── db.container
```

- `<hostname>` is `os.Hostname()` unless `host` is configured
  (see [configuration.md](configuration.md)).
- `common/` is optional.
- On equal relative paths the host file wins.
- If `hosts/<hostname>/` does not exist only `common/` is applied and a
  warning is logged.
- If neither `common/` nor `hosts/<hostname>/` exists the desired tree is
  empty. robbe then refuses to remove every managed file and fails with
  `desired tree is empty; refusing to remove every managed unit (use
  --allow-empty)`; pass `sync --allow-empty` to remove them.
- Files outside `common/` and `hosts/<hostname>/` (a README, other hosts) are
  never applied.

## Ignored files

- Git metadata (names starting with `.git`: `.git/`, `.gitignore`,
  `.gitattributes`, ...) and robbe housekeeping files (names starting with
  `.robbe`). Other dotfiles such as `.env` are applied as support files.
- `*.kube` files. robbe does not manage Kubernetes YAML units; they are
  reported as skipped.

In the target directory, `.robbe-*` files are robbe's own temporary files
(`.robbe-tmp-*`, left by an interrupted atomic write). They are never part of
the managed tree and are cleaned up on the next run. Everything else in the
target is quadlet content; the applied-commit marker lives in the state
directory (see the `state` key in [configuration.md](configuration.md)).

## Unit and support files

A file is a unit file when its extension is one of `.container`, `.pod`,
`.volume`, `.network`, `.image` or `.build`. Everything else (env files,
configuration, drop-in directories such as `nginx.container.d/`) is a support
file.

Unit names are taken from `podman-system-generator --dryrun`, so
`ServiceName=` and the per-type suffixes (`-pod`, `-volume`, `-network`,
`-image`, `-build`) are handled by podman, not by robbe.

## Restart rule

Each `robbe sync` diffs the desired tree against the target directory:

| change                     | action                                   |
|----------------------------|------------------------------------------|
| unit file added            | `systemctl restart <unit>` (starts it when inactive) |
| unit file changed          | `systemctl restart <unit>`               |
| unit file removed          | `systemctl stop <unit>`, then delete     |
| support file added/changed/removed | `systemctl restart` of every managed `.container`/`.pod` unit |

`daemon-reload` runs after the files are written and before any restart.
Added and changed units are passed to systemctl in a single `restart` call
(stops in a single `stop` call) so systemd orders them by their dependencies.

Before the target is touched the staged tree is validated with
`podman-system-generator --dryrun`; on failure nothing is written and robbe
exits with status 1.
