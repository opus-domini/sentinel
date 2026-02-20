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

- If token is enabled, send `Authorization: Bearer <token>`.
- Origin checks apply to all API routes.

## Metadata and Filesystem

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/meta` | Runtime metadata (`tokenRequired`, `defaultCwd`, `version`) |
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
| `POST` | `/api/tmux/sessions/{session}/seen` | Mark seen scope (`pane/window/session`) |

Create payload:

```json
{ "name": "dev", "cwd": "/absolute/path" }
```

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

`/api/tmux/activity/delta` query params:

- `since` (int64 >= 0)
- `limit` (1..1000)

`/api/tmux/timeline` query params:

- `session`, `windowIndex`, `paneId`
- `q`, `severity`, `eventType`
- `since`, `until` (RFC3339)
- `limit` (1..500)

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

### Overview, Alerts and Timeline

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/overview` | Host + Sentinel + services summary |
| `GET` | `/api/ops/metrics` | Host resource metrics (CPU, memory, disk) |
| `GET` | `/api/ops/alerts` | List active/recent ops alerts |
| `POST` | `/api/ops/alerts/{alert}/ack` | Acknowledge one alert |
| `GET` | `/api/ops/timeline` | Ops timeline search/filter |
| `GET` | `/api/ops/config` | Read config file |
| `PATCH` | `/api/ops/config` | Update config file |

Timeline query params:

- `q` text filter
- `severity` (`info`, `warn`, `error`)
- `limit` (1..500)

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
| `POST` | `/api/ops/runbooks` | Create custom runbook |
| `PUT` | `/api/ops/runbooks/{runbook}` | Update runbook |
| `DELETE` | `/api/ops/runbooks/{runbook}` | Delete runbook |
| `POST` | `/api/ops/runbooks/{runbook}/run` | Execute runbook asynchronously (202) |
| `GET` | `/api/ops/jobs/{job}` | Query one runbook job |
| `DELETE` | `/api/ops/jobs/{job}` | Delete a runbook job |

Runbook create/update payload:

```json
{
  "name": "Health Check",
  "description": "Verify service health",
  "enabled": true,
  "webhookURL": "https://hooks.example.com/sentinel",
  "steps": [
    { "type": "command", "title": "Check status", "command": "systemctl --user is-active myapp" },
    { "type": "check", "title": "Verify response", "check": "curl -sf http://localhost:8080/health" },
    { "type": "manual", "title": "Review", "description": "Inspect output above." }
  ]
}
```

The optional `webhookURL` field configures a webhook endpoint that receives a POST with run results on completion. Must be `http` or `https`. See [Runbooks — Webhooks](/features/runbooks.md#webhooks) for payload details.

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
- `ops-timeline`
- `ops-alerts`
- `ops-jobs`
- `all`

## Operations: Guardrails

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/guardrails/rules` | List rules |
| `PATCH` | `/api/ops/guardrails/rules/{rule}` | Upsert one rule |
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
- `GUARDRAIL_BLOCKED`
- `GUARDRAIL_CONFIRM_REQUIRED`
- `RECOVERY_DISABLED`, `RECOVERY_ERROR`
