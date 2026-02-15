# Service and Autoupdate Operations

This page covers managed runtime behavior across Linux and macOS.

## Service Install Behavior

`sentinel service install` is scope-aware:

- Linux + root: system service (`/etc/systemd/system/sentinel.service`)
- Linux + non-root: user service (`~/.config/systemd/user/sentinel.service`)
- macOS + root: system launchd daemon (`/Library/LaunchDaemons/io.opusdomini.sentinel.plist`)
- macOS + non-root: user launch agent (`~/Library/LaunchAgents/io.opusdomini.sentinel.plist`)

Unified service name is `sentinel` from CLI perspective.

## Service Commands

```bash
sentinel service install --exec /path/to/sentinel
sentinel service status
sentinel service uninstall
```

## Autoupdate Timer/Agent

```bash
sentinel service autoupdate install
sentinel service autoupdate status
sentinel service autoupdate uninstall
```

`--scope` options:

- `auto` (default): resolves by runtime privileges
- `user`
- `system`
- `launchd` (macOS compatibility alias)

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

## macOS (`launchd`) Notes

- Same CLI command set as Linux.
- Scope resolves automatically based on privileges.
- Logs:
  - user scope: `~/.sentinel/logs/sentinel.out.log`
  - system scope: `/var/log/sentinel/sentinel.out.log`

## Install Script (`install.sh`)

`install.sh` installs binary and immediately starts/restarts the managed service.

Autoupdate enable during install:

```bash
ENABLE_AUTOUPDATE=1 curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

## Update Lifecycle

- Background autoupdate runs `sentinel update apply ...` on schedule.
- Successful apply can restart managed service according to scope and manager.
- Status can be inspected with:

```bash
sentinel update status
sentinel service autoupdate status
```
