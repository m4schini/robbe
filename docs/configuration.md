# Configuration

robbe is configured from a YAML file, environment variables, and built-in
defaults. Environment variables always win over the file, and the file's
values win over the defaults.

## Config file search order

robbe reads one YAML file, `config.yaml`, resolved in this order. The first
hit wins; robbe does not merge multiple files.

1. The file given with `--config <path>`. It must exist and parse; a missing
   or unreadable path is an error.
2. `$XDG_CONFIG_HOME/robbe/config.yaml` (default `~/.config/robbe/config.yaml`).
3. `<dir>/robbe/config.yaml` for each entry of `$XDG_CONFIG_DIRS`, in order
   (default `/etc/xdg/robbe/config.yaml`).
4. `/etc/robbe/config.yaml`.

If no file is found robbe uses defaults and environment variables only. A
file that is found but does not parse is an error, not a silent fallback.

The legacy locations `~/.robbe.yaml`, `/etc/robbe/.robbe.yaml` and
`./.robbe.yaml` are not read.

## XDG variables

The default directories follow the
[XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/).
An XDG variable that is unset, empty or holds a relative path is ignored and
the spec fallback applies (`~/.config`, `~/.cache`, `~/.local/state`,
`/etc/xdg`). `XDG_RUNTIME_DIR` has no spec fallback; see the `runtime` key.

When robbe runs as root (euid 0) the XDG variables are ignored and the
system-wide directories are used: `/etc/robbe`, `/var/cache/robbe`,
`/var/lib/robbe`, `/run/robbe` and `/etc/containers/systemd/robbe`.

## Environment variables

Every key can be set with an environment variable: prefix `ROBBE_`, uppercase,
with `.` replaced by `_`. For example `repo.auth.ssh_key` becomes
`ROBBE_REPO_AUTH_SSH_KEY`. Environment variables override both the config
file and the defaults.

Additionally, `DEVELOPMENT=1` (or any value `strconv.ParseBool` accepts as
true) switches the logger from production to development output. It is not a
`ROBBE_*` variable and is not part of the `Config` struct.

## Keys

| Key | Env var | Default | Description |
|---|---|---|---|
| `repo.url` | `ROBBE_REPO_URL` | *(none, required)* | Git URL of the repository holding the quadlet files. `robbe status`/`sync` fail with `config.ErrRepoURLMissing` if unset. |
| `repo.ref` | `ROBBE_REPO_REF` | `main` | Branch or tag to check out. |
| `repo.auth.ssh_key` | `ROBBE_REPO_AUTH_SSH_KEY` | *(empty)* | Path to an SSH private key file used for git auth. |
| `repo.auth.ssh_key_password` | `ROBBE_REPO_AUTH_SSH_KEY_PASSWORD` | *(empty)* | Passphrase for `repo.auth.ssh_key`, if the key is encrypted. |
| `repo.auth.token` | `ROBBE_REPO_AUTH_TOKEN` | *(empty)* | HTTP token/password used for token auth over HTTPS. |
| `repo.auth.username` | `ROBBE_REPO_AUTH_USERNAME` | *(empty, defaults to `git` when a token is used)* | HTTP username paired with `repo.auth.token`. |
| `host` | `ROBBE_HOST` | result of `os.Hostname()` | Selects `hosts/<host>/` in the repository. |
| `target` | `ROBBE_TARGET` | `$XDG_CONFIG_HOME/containers/systemd/robbe` (non-root), `/etc/containers/systemd/robbe` (root) | Directory the desired quadlet tree is written to. It holds quadlet content only. |
| `cache` | `ROBBE_CACHE` | `$XDG_CACHE_HOME/robbe` (non-root), `/var/cache/robbe` (root) | Directory holding the git clone (`repo/`) and the staging tree (`staging/`). |
| `state` | `ROBBE_STATE` | `$XDG_STATE_HOME/robbe` (non-root), `/var/lib/robbe` (root) | Directory holding `applied`, the record of the last applied commit (hash and timestamp). `robbe sync` short-circuits only when this file names the fetched commit and the target directory exists; a wiped target is re-applied. |
| `runtime` | `ROBBE_RUNTIME` | `$XDG_RUNTIME_DIR/robbe` (non-root), `/run/robbe` (root) | Directory holding the `lock` file. When `XDG_RUNTIME_DIR` is unset for a non-root user the default is empty and robbe logs a warning and uses `<cache>/lock` instead. |
| `generator` | `ROBBE_GENERATOR` | `/usr/lib/systemd/system-generators/podman-system-generator` | Path to the `podman-system-generator` binary used to validate quadlet units. |
| `user` | `ROBBE_USER` | `true` when running as a non-root user (`os.Geteuid() != 0`), `false` when running as root | Selects the systemd user scope (`systemctl --user`, generator `-user`) vs. the system scope. |

## Auth selection

robbe picks one git auth method per sync, based on which fields are set:

1. `repo.auth.ssh_key` is set (points to a file) -> key-based SSH auth using
   that private key (and `repo.auth.ssh_key_password` if the key is
   encrypted).
2. No `ssh_key` and `repo.url` is an `ssh://` or `git@...` URL -> SSH agent
   auth (uses whatever identity `ssh-agent` offers).
3. `repo.auth.token` is set -> HTTP basic auth, with `repo.auth.username`
   as the username, defaulting to `git` when unset.
4. None of the above -> no auth (works for public HTTP(S) repositories).

Known-hosts checking for SSH uses the go-git default location,
`~/.ssh/known_hosts`; robbe does not configure a separate known-hosts file.

## Examples

### SSH key setup (`~/.config/robbe/config.yaml`)

```yaml
repo:
  url: git@github.com:me/infra.git
  ref: main
  auth:
    ssh_key: /home/me/.ssh/id_ed25519
    ssh_key_password: ""

host: alpha
target: /home/me/.config/containers/systemd/robbe
cache: /home/me/.cache/robbe
state: /home/me/.local/state/robbe
runtime: /run/user/1000/robbe
generator: /usr/lib/systemd/system-generators/podman-system-generator
user: true
```

Every path above is the non-root default; only `repo` is required.

### Token setup, environment variables only

```sh
export ROBBE_REPO_URL=https://github.com/me/infra.git
export ROBBE_REPO_REF=main
export ROBBE_REPO_AUTH_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
export ROBBE_REPO_AUTH_USERNAME=me
```

No config file is needed in this case; every other key falls back to its
default.
