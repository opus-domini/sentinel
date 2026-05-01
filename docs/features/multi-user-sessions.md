# Multi-User Sessions

Sentinel supports running tmux sessions as different OS users. On Linux the default path uses `sudo -n systemd-run --user --machine=<user>@.host` so tmux servers inherit the target user's systemd user environment, including variables such as `XDG_RUNTIME_DIR` and `DBUS_SESSION_BUS_ADDRESS`. Each session tracks which user owns it through a session-user registry persisted in SQLite. This is useful for multi-tenant dev environments, CI agents, or managing services that run under dedicated system accounts.

## Configuration

The `[multi_user]` section in `config.toml` controls user switching behavior:

```toml
[multi_user]
# List of OS users allowed as session targets. When empty, any system user is allowed.
# Environment variable: SENTINEL_ALLOWED_USERS (comma-separated)
allowed_users = ["alice", "deploy", "postgres"]

# Allow targeting the root user for sessions.
# Environment variable: SENTINEL_ALLOW_ROOT_TARGET
allow_root_target = false

# Method for switching users: "systemd-run" (Linux default) or "sudo" (non-Linux default).
# Environment variable: SENTINEL_USER_SWITCH_METHOD
user_switch_method = "systemd-run"
```

When `allowed_users` is empty (the default), any system user with UID >= 1000 and an interactive login shell is permitted as a target. System users are loaded at startup from `/etc/passwd` and serve as the source of truth for the session lifetime.

The current process user is always included in the system user list as a fallback, even if `/etc/passwd` parsing yields no results.

## Security Model

Validation follows a two-tier approach:

1. **Allowlist check** -- when `allowed_users` is configured, only those users are accepted.
2. **System user check** -- when no allowlist is configured, the target must appear in the system user list loaded from `/etc/passwd`.

Additional rules:

- **Root is always blocked** unless `allow_root_target = true`. If root appears in `allowed_users` but `allow_root_target` is false, it is silently removed at startup with a warning.
- **Empty system users blocks all switching.** When `/etc/passwd` cannot be read or yields no users, `ValidateTargetUser` returns `ErrNoSystemUsers` and no user switching is possible.
- **All user switch attempts are logged** via `slog` with `action`, `target_user`, `session`, and `source_ip` fields.

Allowlist entries that do not match any system user produce a startup warning but are not removed.

## Usage

### Creating a session

Pass a `user` field in the `POST /api/tmux/sessions` payload:

```json
{
  "name": "deploy-app",
  "cwd": "/opt/app",
  "user": "deploy"
}
```

When `user` is empty or omitted, the session runs as the Sentinel process user (default behavior).

### Auto-suffix on name collision

If the requested session name already exists, the server tries `name-1`, `name-2`, ... up to `name-99`. The response `name` field reflects the final name used. If all variants are taken, the request fails with `ErrKindSessionExists`.

### Session launchers

Reusable session launchers accept the same optional `user` field as direct session creation. Saving a launcher only records the preset; each launch creates a new session owned by that user and uses the same name-collision suffixing rules as direct session creation.

### Window launchers

Window launchers support two user modes via `userMode`:

- `"session"` (default) -- inherits the user from the session context.
- `"fixed"` -- always runs as the user specified in `userValue`, regardless of the session owner.

```json
{
  "name": "psql",
  "command": "psql -U postgres",
  "userMode": "fixed",
  "userValue": "postgres"
}
```

Fixed-user window launchers create the tmux window in the owning session and run the launcher command through the configured user switch method inside that pane. This keeps window management attached to the session while giving the launched process the target user's environment.

### UI indicators

The sidebar shows a user badge on sessions owned by a different user than the Sentinel process user. This makes it easy to distinguish which sessions belong to which OS account at a glance.

## Persistence

Session-user mappings are stored in the `session_users` SQLite table:

| Column | Type | Description |
|--------|------|-------------|
| `session_name` | TEXT | Primary key, matches the tmux session name |
| `user` | TEXT | OS username that owns the session |
| `updated_at` | DATETIME | Last modification timestamp |

Mappings are created on session creation, migrated on session rename, and deleted on session kill. The `ListSessionUsers` query provides the full map for Watchtower and the session list API.

## Watchtower Integration

Watchtower discovers multi-user sessions through a `UserProvider` callback that returns the list of OS users with active sessions. The provider result is cached with a 10-second TTL to avoid excessive store queries.

Pane IDs from multi-user sessions are namespaced as `user:paneID` (e.g., `alice:%42`) to prevent collisions in the pane journal when the same tmux pane index exists under different users.

## Requirements

- `sudo` must be installed. With the Linux default `user_switch_method = "systemd-run"`, the Sentinel process user must be able to run `systemd-run` through sudo without a password:

  ```
  sentinel ALL=(root) NOPASSWD: /usr/bin/systemd-run
  ```

  If you opt into `user_switch_method = "sudo"`, allow direct tmux execution as each target user instead:

  ```
  sentinel ALL=(alice,deploy,postgres) NOPASSWD: /usr/bin/tmux
  ```

  Fixed-user window launchers run the user switch command from inside the owning tmux session, so the session owner also needs the corresponding sudo permission when targeting a different user.

  With `user_switch_method = "systemd-run"`, sudo logs show `systemd-run` and tmux commands because Sentinel starts or inspects tmux through root-mediated user switching. That is expected; the resulting tmux server runs under the target user and inherits the user's systemd environment.

- On Linux, target users need an active systemd user manager. Enable lingering for service-style accounts that are not normally logged in:

  ```bash
  sudo loginctl enable-linger deploy
  ```

- The systemd unit must **not** set `NoNewPrivileges=true` or `SystemCallArchitectures=native`, as these restrict the ability to execute `sudo`.
