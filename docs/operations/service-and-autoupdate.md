# Service and Autoupdate Operations

This page covers managed runtime behavior across Linux and macOS.

## Service Install Behavior

`sentinel service install` is deployment-aware. `--scope auto` preserves the
only installed deployment; a fresh install selects system scope as root and
user scope otherwise. It never silently creates the opposite scope:

- Linux + root: system service (`/etc/systemd/system/sentinel.service`)
- Linux + non-root: user service (`~/.config/systemd/user/sentinel.service`)
- macOS + root: system launchd daemon (`/Library/LaunchDaemons/io.opusdomini.sentinel.plist`)
- macOS + non-root: user launch agent (`~/Library/LaunchAgents/io.opusdomini.sentinel.plist`)

Unified service name is `sentinel` from CLI perspective.

The service definition persists one deployment identity: scope, binary,
configuration and data directory. Fresh system installs use
`/etc/sentinel/config.toml` with data under `/var/lib/sentinel`; fresh user
installs use `~/.sentinel/config.toml` and `~/.sentinel`. A legacy system
configuration under `/root/.sentinel` is copied to the canonical system path
when its service is explicitly reinstalled; its existing data directory is
preserved so the reinstall does not move or recreate runtime state.

## Service Commands

```bash
sentinel service install --scope auto --exec /path/to/sentinel
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
- Scope resolves automatically based on privileges.
- Logs:
  - user scope: `~/.sentinel/logs/sentinel.out.log`
  - system scope: `/var/log/sentinel/sentinel.out.log`

## Install Script (`install.sh`)

`install.sh` installs binary and immediately starts/restarts the managed service.
Set `INSTALL_SCOPE=user|system` for an explicit scope. The installer diagnoses
an existing opposite-scope or ambiguous installation before downloading or
replacing a binary, and rolls the binary back if service activation fails.

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
