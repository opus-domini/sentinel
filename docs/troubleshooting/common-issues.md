# Common Issues

## `watchtower collect failed` in service logs

Watchtower is the internal Tmux activity/unread projection, not a separate
Sentinel module.

Cause:

- `tmux` unavailable or no running server/session context for current runtime user.

Checks:

```bash
which tmux
sentinel doctor
```

The projection discovers account-targeted sessions from Sentinel's persisted
session-to-account registry. If one is not visible, verify that the session was
created through Sentinel, the target appears in the startup OS-account
inventory, and the `[multi_user]` allowlist/root gate permits it.

## Service shows different listen address than config

Cause:

- Service not restarted after config change, or wrong data dir/config file edited.

Checks:

```bash
sentinel doctor
sentinel service status
```

Then restart managed service.

## Linux user-scope autoupdate install fails with DBUS/XDG errors

Typical error references missing `$DBUS_SESSION_BUS_ADDRESS` or `$XDG_RUNTIME_DIR`.

Fix options:

- Run command in active user session.
- Use system scope if root-managed deployment is intended.

## Session appears in idle unexpectedly after creation

If this happens briefly during reconnect/load:

- ensure events WS is connected (`/ws/events`)
- check for token/auth failures
- inspect browser console/network for WS reconnect loops

## Too many API requests (`sessions`, `windows`, `panes`, `delta`)

Expected steady-state is event-driven with minimal polling.

If request volume is high:

- verify `/ws/events` connection stability
- check for auth/origin rejection causing fallback polling
- confirm frontend is using event patch reconciliation

## `401 UNAUTHORIZED` on API or WS

- Re-enter the shared token in the dedicated authentication gate. Settings is
  available only after the gate succeeds.
- For WS, ensure the `sentinel.v1` subprotocol is used and the auth cookie is present.

## `403 UNTRUSTED_PROXY` or `403 ORIGIN_DENIED`

Sentinel checks the browser connection before opening WebSockets. The UI shows
the rejected value, config path, exact `[server]` entry, and a retry action.

- `UNTRUSTED_PROXY`: the direct non-loopback HTTPS proxy address is missing
  from `server.trusted_proxies`. Loopback proxies are trusted automatically.
- `ORIGIN_DENIED`: the browser origin does not match Sentinel and is missing
  from `server.allowed_origins`.

After editing the config, restart the managed service and run:

```bash
sentinel doctor
```

## Mobile scroll/keyboard instability

Sentinel uses viewport tracking and touch lock zones.

If layout drifts:

- reload page after orientation change
- confirm latest frontend assets are served
- test PWA mode for more stable viewport behavior

## `403 USER_NOT_ALLOWED` on OS-account session create

Cause:

- The target OS account is not in the `[multi_user]` allowlist, or startup
  account inventory validation rejected it.

Checks:

- Verify the effective `[multi_user].allowed_users`, or confirm that the target
  has UID >= 1000 and an interactive shell in `/etc/passwd`.
- If targeting root, ensure `allow_root_target = true` is set.

## `ErrNoSystemUsers` preventing OS account targeting

Cause:

- Sentinel could not build an OS-account inventory from `/etc/passwd` or the
  current-process fallback.

Checks:

- Verify `/etc/passwd` is readable by the sentinel process user.
- Run `sentinel doctor` to confirm system user detection.

## `sudo` failures for OS-account targeting

Cause:

- `sudo` is not installed, or NOPASSWD is not configured for the sentinel user.

Checks:

```bash
which sudo
sudo -l -U <sentinel-user>
```

Ensure the sentinel user has a NOPASSWD sudoers entry for the required commands.

## Useful Diagnostics

```bash
sentinel doctor
sentinel service status
sentinel service autoupdate status
sentinel update status
```
