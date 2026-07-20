# CLI Reference

## Root Commands

```bash
sentinel config <init|edit|path|validate|show>
sentinel db <init|status|reset>
sentinel doctor
sentinel daemon
sentinel service <install|migrate|uninstall|status|logs|autoupdate>
sentinel update <check|apply|status>
sentinel completion <bash|zsh|fish>
sentinel --help
sentinel --version | -v | version
```

Running `sentinel` with no arguments prints this help; the server starts only
via the explicit `daemon` command.

Global flag:

- `--config <path>`: use a specific config file path for the invocation.

Managed `config` commands resolve the installed deployment automatically.
`sentinel config --scope system ...` selects the system deployment explicitly.

## `sentinel config`

### Init

Create the canonical config file.

```bash
sentinel config init
sentinel config init --force
```

The command writes `config.toml` under the active data dir. Existing files are
kept intact unless `--force` is set.

### Edit

```bash
sentinel config edit
EDITOR="code --wait" sentinel config edit
```

Ensures `config.toml` exists, opens it with `$EDITOR`, `$VISUAL`, or `xdg-open`,
and validates the file after blocking editors close. When `xdg-open` returns
immediately, run `sentinel config validate` after saving.

### Path

```bash
sentinel config path
```

Prints the canonical config file path.

For an installed system deployment this prints `/etc/sentinel/config.toml`,
even when the command is executed through `sudo`.

### Validate

```bash
sentinel config validate
sentinel config validate --effective
```

Validates the config file before a service restart or daemon start.
`--effective` validates the values after environment overrides and is also the
contract used by the updater to check a candidate binary before installation.

### Show

```bash
sentinel config show
```

Prints the effective configuration as JSON after applying defaults, file values
and environment overrides. Secret values such as `token` are redacted.

## `sentinel db`

### Init

Create the canonical config file when missing, ensure the data directory exists,
and initialize the SQLite database with migrations.

```bash
sentinel db init
```

### Status

```bash
sentinel db status
```

Prints the resolved database path, database/WAL/SHM file sizes, and row counts
for flushable runtime storage resources.

### Reset

```bash
sentinel db reset --yes
sentinel db reset --yes --resource ops-jobs
sentinel db reset --yes --force
```

Without `--force`, flushes derived runtime storage through the same resource
model exposed in the Settings storage panel. This conservative reset preserves
configuration, presets, runbooks, schedules, custom services
and other durable setup data.

With `--force`, deletes `sentinel.db` and its SQLite sidecar files, then
recreates the database by running migrations. This wipes all state stored in the
database while leaving `config.toml` intact.

## `sentinel daemon`

Start HTTP server using config/env values.

```bash
sentinel daemon
```

## `sentinel service`

### Migrate

```bash
sudo sentinel service migrate --scope system
```

Migrates the historical root-home system layout to `/etc/sentinel`,
`/var/lib/sentinel` and `/var/log/sentinel`. Divergent active and legacy config
files are reported as a conflict and are never selected or overwritten
silently.

### Install

```bash
sentinel service install --scope auto|user|system --exec PATH --enable=true --start=true
```

Flags:

- `--exec`: binary path for service unit (optional, defaults to current executable).
- `--scope`: preserve the existing scope with `auto`, or explicitly select
  `user`/`system`. A fresh `auto` install is rejected so caller privileges can
  never decide deployment intent implicitly.
- `--enable`: enable at boot/login.
- `--start`: start immediately.

### Uninstall

```bash
sentinel service uninstall --disable=true --stop=true --remove-unit=true
sentinel service uninstall --scope auto|user|system --purge
```

Flags:

- `--disable`: disable auto-start.
- `--stop`: stop service.
- `--remove-unit`: remove managed unit/plist.
- `--purge`: also remove the autoupdate timer, the bash completion and the
  deployment binary. The binary is retained if another deployment uses it.
  Runtime data is left intact.

`--purge` is the full teardown — it is what `make uninstall` runs.

When both scopes are present, the ambiguity error reports the detected unit
paths and keeps `auto` disabled until one deployment is removed. A root login
may remove a legacy user-scoped deployment owned by root with:

```bash
sentinel service uninstall --scope user --purge
```

This exception applies only to removal. Root still cannot install, update, or
operate a user-scoped deployment, and a process invoked through `sudo` cannot
remove the invoking user's deployment; run that command as the owning user.

### Status

```bash
sentinel service status
```

Reports the managed service in **every scope where it is installed** — user and
system — regardless of the euid the command runs under. A system service
installed with `sudo` is still reported when `status` is run as a normal user.
Prints the unit, binary, config and data paths plus manager availability and
enabled/active state per scope.

`uninstall`, `logs` and the lifecycle commands (`start`/`stop`/`restart`/
`enable`/`disable`) likewise act on the scope the service is actually installed
in; modifying a system-scope service still requires `sudo`.

### Logs

```bash
sentinel service logs --scope auto|user|system --follow --lines 50
```

Flags:

- `--follow`, `-f`: stream new log lines as they arrive.
- `--lines`, `-n`: number of past log lines to show (default 50).

Streams the managed service log: `journalctl` for the systemd unit on Linux, the launchd plist log files on macOS.

### Lifecycle

```bash
sentinel service start
sentinel service stop
sentinel service restart
sentinel service enable
sentinel service disable
```

Drive the managed unit directly: `start`, `stop` and `restart` control the
running service; `enable` and `disable` control whether it launches on
boot/login. These map to `systemctl` on Linux and `launchctl` on macOS.
All lifecycle commands accept `--scope auto|user|system`.

## `sentinel service autoupdate`

### Install timer/agent

```bash
sentinel service autoupdate install \
  --enable=true \
  --start=true \
  --scope auto|user|system \
  --on-calendar daily \
  --randomized-delay 1h
```

### Uninstall

```bash
sentinel service autoupdate uninstall \
  --disable=true --stop=true --remove-unit=true \
  --scope auto|user|system
```

### Status

```bash
sentinel service autoupdate status --scope auto|user|system
```

## `sentinel doctor`

```bash
sentinel doctor
```

Outputs host/runtime diagnosis: OS/arch, config path and validity, allowed
origins, trusted proxies, listen addr, data dir, tmux path, service manager
status, and managed unit states. Every problem is printed with its concrete
cause, and the command exits non-zero when any problem is found.

## `sentinel update`

### Check

```bash
sentinel update check --scope auto|user|system --repo owner/name --api URL --os linux --arch amd64
```

### Apply

```bash
sentinel update apply \
  --repo owner/name \
  --api URL \
  --exec PATH \
  --os linux \
  --arch amd64 \
  --allow-downgrade=false \
  --allow-unverified=false \
  --restart=true \
  --scope auto|user|system
```

Successful applies restart the managed service by default so the new binary is
the running binary. Before loading configuration or downloading a release, the
command detects whether the installed service is in user or system scope. A
normal user invoking apply against a system installation is stopped immediately
with `sudo sentinel update apply --scope system`, instead of reading the user's
unrelated config or failing later while writing the system binary. Use
`--scope user` or `--scope system` to select explicitly when both installations
exist; `auto` rejects that ambiguous state. Use `--restart=false` only when you
intentionally want to install the binary now and restart Sentinel yourself
later.

Before replacing the installed executable, the updater runs the downloaded
candidate against the current effective configuration. An incompatible release
is rejected while the running binary remains untouched.
If restart or the post-restart health check fails, the updater reports whether
the previous binary was actually restored and restarted; rollback failures are
never described as successful.

### Status

```bash
sentinel update status --scope auto|user|system
```

## `sentinel completion`

Print a shell completion script to stdout.

```bash
sentinel completion <bash|zsh|fish>
```

`make install` and `install.sh` install the bash completion automatically.
To install it manually:

```bash
# bash
sentinel completion bash > ~/.local/share/bash-completion/completions/sentinel

# zsh (ensure the directory is on $fpath)
sentinel completion zsh > "${fpath[1]}/_sentinel"

# fish
sentinel completion fish > ~/.config/fish/completions/sentinel.fish
```

Open a new shell after installing for completion to take effect.

## Exit Codes

- `0`: success
- `1`: runtime/operation error
- `2`: invalid CLI usage or flags
