# Service and Autoupdate Operations

This page covers managed runtime behavior across Linux and macOS.

## Service Install Behavior

`sentinel service install` is deployment-aware. `--scope auto` preserves the
only installed deployment or canonical standalone binary. A fresh install
requires `--scope user|system`; it never infers intent from privileges or
silently creates the opposite scope:

- Linux + root: system service (`/etc/systemd/system/sentinel.service`)
- Linux + non-root: user service (`~/.config/systemd/user/sentinel.service`)
- macOS + root: system launchd daemon (`/Library/LaunchDaemons/io.opusdomini.sentinel.plist`)
- macOS + non-root: user launch agent (`~/Library/LaunchAgents/io.opusdomini.sentinel.plist`)

Unified service name is `sentinel` from CLI perspective.

The service definition persists one deployment identity: scope, binary,
configuration and data directory. Linux system installs use
`/etc/sentinel/config.toml`, `/var/lib/sentinel` and
`/var/log/sentinel/sentinel.log`; user installs keep all three resources under
`~/.sentinel`.

A deployment is either canonical or legacy; hybrid layouts are invalid.
`service install` and `update apply` refuse a legacy or hybrid deployment and
point to the migration command instead of silently changing one path:

```bash
sudo sentinel service migrate --scope system
```

Migration stops an active service before copying SQLite state, rebases the
default database and log paths, rewrites the service definition, restarts it in
its previous state and removes the legacy directory only after success. If the
active and legacy TOML files differ, migration stops before changing anything
and requires explicit reconciliation.

## Service Commands

```bash
sentinel service install --scope auto --exec /path/to/sentinel
sentinel service migrate --scope system
sentinel service status
sentinel service restart --scope auto
sentinel service uninstall --scope auto
```

## Autoupdate Timer/Agent

```bash
sentinel service autoupdate install
sentinel service autoupdate status
sentinel service autoupdate uninstall
```

`--scope` options are `auto`, `user`, and `system`:

- `auto` (default): selects the only installed deployment and rejects an
  ambiguous user+system installation.
- `user` or `system`: select one deployment explicitly.

Autoupdate derives its binary, config and service manager from that deployment;
these values cannot drift independently.
Reinstalling an existing deployment must use the binary recorded in its unit.
To move a deployment to another binary path or scope, uninstall it first and
then install the new deployment explicitly.

## Linux (`systemd`) Notes

### User scope requirements

`systemctl --user` requires active user bus and runtime dir.

Typical issue:

- `Failed to connect to user scope bus ... $DBUS_SESSION_BUS_ADDRESS and $XDG_RUNTIME_DIR not defined`

Resolution:

- Install in active user session, or
- Use root/system scope when appropriate, or
- Ensure user lingering/session is configured.

### Root scope

- `systemctl status sentinel`
- `journalctl -u sentinel -f`

> ⚠️ Do not add `NoNewPrivileges=true` or `SystemCallArchitectures=native` to the systemd unit when multi-user sessions are in use — `sudo` requires the new-privilege capability.

## macOS (`launchd`) Notes

- Same CLI command set as Linux.
- Existing scope resolves automatically; fresh installs require a choice.
- Logs:
  - user scope: `~/.sentinel/logs/sentinel.out.log`
  - system scope: `/var/log/sentinel/sentinel.out.log`

## Install Script (`install.sh`)

`make install` and `install.sh` install the binary and immediately
start/restart the managed service. Both detect an existing managed service
first and preserve its scope. If no service exists, they also inspect the
canonical binary locations for a standalone installation.

A fresh interactive install explains the user and system layouts and requires
an explicit choice; it never infers scope from whether the installer happened
to run as root. A fresh non-interactive install must set
`INSTALL_SCOPE=user|system`. Choosing system scope from an unprivileged shell
requests `sudo` only for system files and service management. User scope never
runs through `sudo`.

The printed next steps retain the resolved scope. System deployments use
`sudo sentinel doctor` and `sudo sentinel update apply --scope system`; user
deployments use the corresponding unprivileged user commands. Systemd units
under `/etc/systemd/system` are world-readable (`0644`) so a normal user can
inspect deployment status without gaining access to the root-only Sentinel
configuration.

The downloaded or locally built Go CLI is the single source of truth for scope
discovery, conflicts and the interactive prompt. The shell installer only
downloads/verifies artifacts and executes the resulting plan. Conflicts are
diagnosed before replacing a binary, and service activation failure rolls the
binary back.

When the main service is reinstalled or upgraded, Sentinel also refreshes an
existing autoupdate definition in the same scope. The refresh keeps the current
timer schedule and activation state while updating the executable, canonical
paths, CLI arguments and unit permissions. It does not enable autoupdate when
no updater is already installed.

Autoupdate enable during install:

```bash
ENABLE_AUTOUPDATE=1 curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

## Update Lifecycle

- Background autoupdate runs `sentinel update apply --scope user|system` on schedule.
- Before loading configuration or downloading a release, manual apply detects
  whether Sentinel is installed in user or system scope. A normal user pointed
  at a system installation is stopped with the exact `sudo sentinel update
  apply --scope system` recovery command.
- Updates refuse noncanonical managed paths until `service migrate` completes;
  this prevents an update from preserving or deepening a hybrid deployment.
- Successful apply restarts the exact managed service selected from the unit.
  If units exist in both scopes, manual apply requires an explicit
  `--scope user` or `--scope system` choice.
- Manual `sentinel update apply --scope user|system` overrides the auto scope
  decision when the update process needs to target a specific service manager.
- Manual `sentinel update apply --restart=false` installs the binary without
  restarting for maintenance-window edge cases.
- Every apply validates the current effective configuration with the candidate
  binary before replacing the installed executable.
- A standalone binary with no managed service can still update itself, but no
  service restart is attempted.
- Status can be inspected with:

```bash
sentinel update status
sentinel service autoupdate status
```

`sentinel doctor` includes an installed updater in its service checks and
reports a failed last run. Re-running `sentinel service install --scope
user|system` repairs the existing updater without replacing its schedule.
