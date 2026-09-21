# systemd deployment

robbe runs natively on the host as a systemd oneshot service driven by a
timer. It needs `systemctl`, the quadlet directory and
`podman-system-generator`, so it is not shipped as a container.

`robbe install` writes the same two units as in this directory, with the
path of the running binary and the interval from `--interval` (default
`10s`), reloads systemd and runs `enable --now robbe-sync.timer`.
`robbe uninstall` reverses that. The files here are the reference copies for
a manual installation; `make check-units` keeps them in step with the
embedded template.

## Directories

The service declares `ConfigurationDirectory=`, `CacheDirectory=`,
`StateDirectory=` and `RuntimeDirectory=robbe`, so systemd creates the
directories robbe uses before each run, with the right owner and mode:

| directive                | system (root)      | user instance                |
|--------------------------|--------------------|------------------------------|
| `ConfigurationDirectory` | `/etc/robbe`       | `$XDG_CONFIG_HOME/robbe`     |
| `CacheDirectory`         | `/var/cache/robbe` | `$XDG_CACHE_HOME/robbe`      |
| `StateDirectory`         | `/var/lib/robbe`   | `$XDG_STATE_HOME/robbe`      |
| `RuntimeDirectory`       | `/run/robbe`       | `$XDG_RUNTIME_DIR/robbe`     |

These match robbe's defaults (see `docs/configuration.md`). The runtime
directory is removed when the oneshot service stops; the lock inside it only
lives as long as the process. A manual `robbe sync` outside the unit creates
the same directories itself.

## Rootless (user instance)

```sh
install -Dm755 robbe ~/.local/bin/robbe
install -Dm644 -t ~/.config/systemd/user/ robbe-sync.service robbe-sync.timer
sed -i 's#/usr/local/bin/robbe#'"$HOME"'/.local/bin/robbe#' ~/.config/systemd/user/robbe-sync.service
install -d ~/.config/robbe
cat > ~/.config/robbe/config.yaml <<YAML
repo:
  url: git@github.com:me/infra.git
YAML
systemctl --user daemon-reload
systemctl --user enable --now robbe-sync.timer
```

The user instance stops with the last login session unless lingering is
enabled, so the timer only fires while you are logged in until you run:

```sh
loginctl enable-linger "$USER"
```

Quadlets are written to `~/.config/containers/systemd/robbe/`.

## System (root)

```sh
install -Dm755 robbe /usr/local/bin/robbe
install -Dm644 -t /etc/systemd/system/ robbe-sync.service robbe-sync.timer
install -Dm600 /dev/null /etc/robbe/config.yaml   # then add repo.url, see docs/configuration.md
systemctl daemon-reload
systemctl enable --now robbe-sync.timer
```

Quadlets are written to `/etc/containers/systemd/robbe/`.

## Checking

```sh
systemctl [--user] list-timers robbe-sync.timer
journalctl [--user] -u robbe-sync.service -n 50
robbe status
```
