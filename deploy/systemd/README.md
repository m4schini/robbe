# systemd deployment

robbe runs natively on the host as a systemd oneshot service driven by a
timer. It needs `systemctl`, the quadlet directory and
`podman-system-generator`, so it is not shipped as a container.

`robbe install` writes the same two units as in this directory, with the
path of the running binary and the interval from `--interval` (default
`5m`), reloads systemd and runs `enable --now robbe-sync.timer`.
`robbe uninstall` reverses that. The files here are the reference copies for
a manual installation.

## Rootless (user instance)

```sh
install -Dm755 robbe ~/.local/bin/robbe
install -Dm644 -t ~/.config/systemd/user/ robbe-sync.service robbe-sync.timer
sed -i 's#/usr/local/bin/robbe#'"$HOME"'/.local/bin/robbe#' ~/.config/systemd/user/robbe-sync.service
cat > ~/.robbe.yaml <<YAML
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
install -Dm600 /dev/null /etc/robbe/.robbe.yaml   # then add repo.url, see docs/configuration.md
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
