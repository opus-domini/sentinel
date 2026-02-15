# Common Issues

## `watchtower collect failed` in service logs

Cause:

- `tmux` unavailable or no running server/session context for current runtime user.

Checks:

```bash
which tmux
sentinel doctor
```

If service runs as root, tmux sessions created by other users will not be visible by default.

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

- Check token value in Settings and client headers.
- For WS, ensure subprotocol includes `sentinel.auth.<base64url-token>`.

## Mobile scroll/keyboard instability

Sentinel uses viewport tracking and touch lock zones.

If layout drifts:

- reload page after orientation change
- confirm latest frontend assets are served
- test PWA mode for more stable viewport behavior

## Recovery restore fails

Use:

```bash
sentinel recovery list
sentinel recovery restore --snapshot <id> --wait=true
```

If restore conflicts, adjust `--conflict rename|replace|skip` and `--mode`.

## Useful Diagnostics

```bash
sentinel doctor
sentinel service status
sentinel service autoupdate status
sentinel update status
```
