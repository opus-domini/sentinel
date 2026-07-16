# Common Issues

## `watchtower collect failed` in service logs

Cause:

- `tmux` unavailable or no running server/session context for current runtime user.

Checks:

```bash
which tmux
sentinel doctor
```

Watchtower now discovers multi-user sessions automatically via the `UserProvider` callback. If sessions are still not visible, verify that `[multi_user]` is configured and the target user exists on the system.

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

- Check token value in Settings — authentication uses an HttpOnly cookie set via the UI.
- For WS, ensure the `sentinel.v1` subprotocol is used and the auth cookie is present.

## `403 UNTRUSTED_PROXY` or `403 ORIGIN_DENIED`

Sentinel checks the browser connection before opening WebSockets. The UI shows
the rejected value, config path, exact `[server]` entry, and a retry action.

- `UNTRUSTED_PROXY`: the direct HTTPS proxy address is missing from
  `server.trusted_proxies`.
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

## `403 USER_NOT_ALLOWED` on session create

Cause:

- The target user is not in the `[multi_user]` allowlist, or system user validation rejected the user.

Checks:

- Verify `[multi_user]` section exists in config with correct `allowed_users` or that the target user has UID >= 1000 in `/etc/passwd`.
- If targeting root, ensure `allow_root_target = true` is set.

## `ErrNoSystemUsers` preventing user switching

Cause:

- System users could not be loaded from `/etc/passwd`.

Checks:

- Verify `/etc/passwd` is readable by the sentinel process user.
- Run `sentinel doctor` to confirm system user detection.

## `sudo` failures for multi-user sessions

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
