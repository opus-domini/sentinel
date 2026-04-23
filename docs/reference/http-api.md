# HTTP API Reference

All endpoints are JSON.

## Response Envelope

Success:

```json
{
  "data": {}
}
```

Error:

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "...",
    "details": {}
  }
}
```

## Auth and Origin

When token is configured, auth uses HttpOnly cookies:

1. Client sends `PUT /api/auth/token` with `{"token":"..."}`.
2. Server validates and sets HttpOnly cookie `sentinel_auth`.
3. All subsequent requests are authenticated via this cookie.

Origin checks apply to all API routes.

## Auth Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `PUT` | `/api/auth/token` | Set auth cookie |
| `DELETE` | `/api/auth/token` | Clear auth cookie |

`PUT /api/auth/token` payload:

```json
{ "token": "..." }
```

## Metadata and Filesystem

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/meta` | Runtime metadata (`tokenRequired`, `defaultCwd`, `version`, `timezone`, `locale`, `hostname`, `processUser`, `isRoot`, `canSwitchUser`, `allowedUsers`) |
| `GET` | `/api/fs/dirs` | Directory suggestions for session creation |

`/api/fs/dirs` query params: `prefix`, `limit`.

## Tmux Sessions

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tmux/sessions` | List sessions (enriched projection) |
| `POST` | `/api/tmux/sessions` | Create session |
| `PATCH` | `/api/tmux/sessions/{session}` | Rename session |
| `PATCH` | `/api/tmux/sessions/{session}/icon` | Set session icon |
| `DELETE` | `/api/tmux/sessions/{session}` | Kill session |
| `PATCH` | `/api/tmux/sessions/order` | Reorder pinned sessions |
| `POST` | `/api/tmux/sessions/{session}/seen` | Mark seen scope (`pane/window/session`) |

Create payload:

```json
{ "name": "dev", "cwd": "/absolute/path", "icon": "rocket", "user": "deploy" }
```

`icon` and `user` are optional. On name collision the server tries `name-1` through `name-9`, so the response `name` may differ from the requested name.

## Tmux Launchers

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tmux/launchers` | List launchers |
| `POST` | `/api/tmux/launchers` | Create launcher |
| `PATCH` | `/api/tmux/launchers/order` | Reorder launchers |
| `PATCH` | `/api/tmux/launchers/{launcher}` | Update launcher |
| `DELETE` | `/api/tmux/launchers/{launcher}` | Delete launcher |
| `POST` | `/api/tmux/sessions/{session}/launchers/{launcher}/launch` | Launch a window from launcher |

## Session Presets

The tmux sidebar exposes these presets as session launchers from the session `+` split button.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tmux/session-presets` | List session presets |
| `POST` | `/api/tmux/session-presets` | Create session preset |
| `PATCH` | `/api/tmux/session-presets/order` | Reorder presets |
| `PATCH` | `/api/tmux/session-presets/{preset}` | Update preset |
| `DELETE` | `/api/tmux/session-presets/{preset}` | Delete preset |
| `POST` | `/api/tmux/session-presets/{preset}/launch` | Launch session from preset |

## Tmux Windows and Panes

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tmux/sessions/{session}/windows` | List windows |
| `GET` | `/api/tmux/sessions/{session}/panes` | List panes |
| `POST` | `/api/tmux/sessions/{session}/select-window` | Select window |
| `POST` | `/api/tmux/sessions/{session}/select-pane` | Select pane |
| `POST` | `/api/tmux/sessions/{session}/new-window` | Create window |
| `POST` | `/api/tmux/sessions/{session}/kill-window` | Kill window |
| `POST` | `/api/tmux/sessions/{session}/kill-pane` | Kill pane |
| `POST` | `/api/tmux/sessions/{session}/split-pane` | Split pane |
| `POST` | `/api/tmux/sessions/{session}/rename-window` | Rename window |
| `POST` | `/api/tmux/sessions/{session}/rename-pane` | Rename pane |

Split payload:

```json
{ "paneId": "%3", "direction": "vertical" }
```

Direction: `vertical` or `horizontal`.

## Activity and Timeline

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tmux/activity/delta` | Delta patches by global revision |
| `GET` | `/api/tmux/activity/stats` | Watchtower runtime metrics |
| `GET` | `/api/tmux/timeline` | Search timeline events |
| `GET` | `/api/tmux/frequent-dirs` | Frequently used directories |

`/api/tmux/activity/delta` query params:

- `since` (int64 >= 0)
- `limit` (1..1000)

`/api/tmux/timeline` query params:

- `session`, `windowIndex`, `paneId`
- `q`, `severity`, `eventType`
- `since`, `until` (RFC3339)
- `limit` (1..500)

`/api/tmux/frequent-dirs` query params:

- `limit` (1..20, default 5)

## Presence

| Method | Path | Purpose |
| --- | --- | --- |
| `PUT` | `/api/tmux/presence` | Upsert terminal presence (HTTP fallback) |

Payload:

```json
{
  "terminalId": "...",
  "session": "dev",
  "windowIndex": 1,
  "paneId": "%4",
  "visible": true,
  "focused": true
}
```

## Operations: Control Plane

### Overview, Alerts and Activity

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/overview` | Host + Sentinel + services summary |
| `GET` | `/api/ops/metrics` | Host resource metrics (CPU, memory, disk) |
| `GET` | `/api/ops/alerts` | List active/recent ops alerts |
| `POST` | `/api/ops/alerts/bulk-ack` | Bulk-acknowledge alerts (max 100) |
| `POST` | `/api/ops/alerts/{alert}/ack` | Acknowledge one alert |
| `DELETE` | `/api/ops/alerts/{alert}` | Delete one alert |
| `GET` | `/api/ops/activity` | Ops activity search/filter |
| `GET` | `/api/ops/config` | Read config file |
| `PATCH` | `/api/ops/config` | Update config file |

Activity query params:

- `q` text filter
- `source` activity source filter
- `severity` (`info`, `warn`, `error`)
- `limit` (1..500)

Bulk-ack payload:

```json
{ "ids": [1, 2, 3] }
```

`ids` must contain 1–100 alert IDs.

### Services

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/services` | Tracked service list and runtime status |
| `GET` | `/api/ops/services/browse` | Browse all host units with tracked status |
| `GET` | `/api/ops/services/discover` | Discover available services |
| `POST` | `/api/ops/services` | Register custom service |
| `DELETE` | `/api/ops/services/{service}` | Unregister custom service |
| `POST` | `/api/ops/services/{service}/action` | Execute `start`, `stop`, or `restart` |
| `GET` | `/api/ops/services/{service}/status` | Detailed manager status for one service |
| `GET` | `/api/ops/services/{service}/logs` | Service logs |
| `POST` | `/api/ops/services/unit/action` | Act on unit directly by name |
| `GET` | `/api/ops/services/unit/status` | Inspect unit directly |
| `GET` | `/api/ops/services/unit/logs` | Unit logs directly |

Service action payload:

```json
{ "action": "restart" }
```

Custom service registration payload:

```json
{
  "name": "myapp",
  "displayName": "My App",
  "manager": "systemd",
  "unit": "myapp.service",
  "scope": "user"
}
```

Unit action payload:

```json
{
  "unit": "myapp.service",
  "scope": "user",
  "manager": "systemd",
  "action": "restart"
}
```

Unit query params (status and logs): `unit`, `scope`, `manager`, `lines`.

### Runbooks

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/runbooks` | List runbooks and recent jobs |
| `GET` | `/api/ops/runbooks/suggest` | Suggest runbooks for a marker/session |
| `POST` | `/api/ops/runbooks` | Create custom runbook |
| `PUT` | `/api/ops/runbooks/{runbook}` | Update runbook |
| `DELETE` | `/api/ops/runbooks/{runbook}` | Delete runbook |
| `POST` | `/api/ops/runbooks/{runbook}/run` | Execute runbook asynchronously (202) |
| `GET` | `/api/ops/jobs/{job}` | Query one runbook job |
| `DELETE` | `/api/ops/jobs/{job}` | Delete a runbook job |
| `POST` | `/api/ops/runs/{runId}/approve` | Approve a waiting approval step (202) |
| `POST` | `/api/ops/runs/{runId}/reject` | Reject a waiting approval step |

`/api/ops/runbooks/suggest` query params: `marker`, `session`.

Runbook create/update payload:

```json
{
  "name": "Health Check",
  "description": "Verify service health",
  "enabled": true,
  "webhookURL": "https://hooks.example.com/sentinel",
  "steps": [
    { "type": "run", "title": "Check status", "command": "systemctl --user is-active myapp" },
    { "type": "run", "title": "Verify response", "command": "curl -sf http://localhost:8080/health" },
    { "type": "script", "title": "Rotate logs", "script": "#!/bin/bash\ncd /var/log && gzip *.log" },
    { "type": "approval", "title": "Review", "description": "Inspect output above." }
  ]
}
```

Step types:

- `run` — execute a single shell command (`command` field).
- `script` — execute a multi-line script (`script` field).
- `approval` — pause and wait for manual approval (`description` field).

Per-step options (all optional):

| Field | Type | Description |
| --- | --- | --- |
| `continueOnError` | bool | Continue to the next step on failure |
| `timeout` | int | Step timeout in seconds |
| `retries` | int | Number of retry attempts |
| `retryDelay` | int | Delay between retries in seconds |

The optional `webhookURL` field configures a webhook endpoint that receives a POST with run results on completion. Must be `http` or `https`. See [Runbooks — Webhooks](/features/runbooks.md#webhooks) for payload details.

### Schedules

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/schedules` | List schedules |
| `POST` | `/api/ops/schedules` | Create schedule |
| `PUT` | `/api/ops/schedules/{schedule}` | Update schedule |
| `DELETE` | `/api/ops/schedules/{schedule}` | Delete schedule |
| `POST` | `/api/ops/schedules/{schedule}/trigger` | Trigger schedule immediately |

### Markers

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/markers` | List marker patterns |
| `PUT` | `/api/ops/markers/{pattern}` | Create or update marker pattern |
| `DELETE` | `/api/ops/markers/{pattern}` | Delete marker pattern |

Marker upsert payload:

```json
{
  "pattern": "error|fail",
  "severity": "error",
  "label": "Error marker",
  "enabled": true,
  "priority": 10
}
```

`pattern` and `enabled` are required. When `{pattern}` in the URL path is omitted, a random ID is generated.

### Settings

| Method | Path | Purpose |
| --- | --- | --- |
| `PATCH` | `/api/ops/settings/timezone` | Update timezone |
| `PATCH` | `/api/ops/settings/locale` | Update locale |
| `GET` | `/api/ops/settings/webhook` | Get webhook configuration |
| `PATCH` | `/api/ops/settings/webhook` | Update webhook configuration |
| `POST` | `/api/ops/webhook/test` | Test webhook delivery |

## Operations: Storage

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/storage/stats` | Storage usage by resource |
| `POST` | `/api/ops/storage/flush` | Flush resource data |

Flush payload:

```json
{ "resource": "timeline" }
```

Allowed resources:

- `timeline`
- `activity-journal`
- `guardrail-audit`
- `recovery-history`
- `ops-activity`
- `ops-alerts`
- `ops-jobs`
- `all`

## Operations: Guardrails

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/guardrails/rules` | List rules |
| `POST` | `/api/ops/guardrails/rules` | Create rule |
| `PATCH` | `/api/ops/guardrails/rules/{rule}` | Upsert one rule |
| `DELETE` | `/api/ops/guardrails/rules/{rule}` | Delete rule |
| `GET` | `/api/ops/guardrails/audit` | List audit events |
| `POST` | `/api/ops/guardrails/evaluate` | Evaluate one action/command manually |

## Recovery

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/recovery/overview` | Recovery status summary |
| `GET` | `/api/recovery/sessions` | List killed sessions |
| `POST` | `/api/recovery/sessions/{session}/archive` | Archive recovery session |
| `GET` | `/api/recovery/sessions/{session}/snapshots` | List snapshots |
| `GET` | `/api/recovery/snapshots/{snapshot}` | Snapshot details |
| `POST` | `/api/recovery/snapshots/{snapshot}/restore` | Start restore job |
| `GET` | `/api/recovery/jobs/{job}` | Job progress/state |

Restore payload (optional body):

```json
{
  "mode": "confirm",
  "conflictPolicy": "rename",
  "targetSession": "target-name"
}
```

## Common Error Codes

- `INVALID_REQUEST`
- `UNAUTHORIZED`
- `ORIGIN_DENIED`
- `STORE_ERROR`
- `UNAVAILABLE`
- `TMUX_*` (`TMUX_NOT_FOUND`, `SESSION_NOT_FOUND`, etc.)
- `OPS_RUNBOOK_NOT_FOUND`, `OPS_JOB_NOT_FOUND`
- `OPS_ALERT_NOT_FOUND`, `SCHEDULE_NOT_FOUND`
- `GUARDRAIL_BLOCKED`
- `GUARDRAIL_CONFIRM_REQUIRED`
- `RECOVERY_DISABLED`, `RECOVERY_ERROR`
- `USER_NOT_ALLOWED` — 403 — Target user not in allowlist or system users
- `TMUX_LAUNCHER_NOT_FOUND` — 404 — Referenced launcher does not exist
- `TMUX_LAUNCHER_EXISTS` — 409 — Launcher with this name already exists
- `INVALID_STATE` — 409 — Operation not valid in the current state (e.g., runbook step approve/reject)
