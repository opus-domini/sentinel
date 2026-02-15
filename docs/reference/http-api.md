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

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/ops/overview` | Host + Sentinel + services summary |
| `GET` | `/api/ops/services` | Service list and runtime status |
| `POST` | `/api/ops/services/{service}/action` | Execute `start`, `stop`, or `restart` |
| `GET` | `/api/ops/alerts` | List active/recent ops alerts |
| `POST` | `/api/ops/alerts/{alert}/ack` | Acknowledge one alert |
| `GET` | `/api/ops/timeline` | Ops timeline search/filter |
| `GET` | `/api/ops/runbooks` | List runbooks and latest jobs |
| `POST` | `/api/ops/runbooks/{runbook}/run` | Execute runbook asynchronously |
| `GET` | `/api/ops/jobs/{job}` | Query one runbook execution |

Service action payload:

```json
{ "action": "restart" }
```

Timeline query params:

- `q` text filter
- `severity` (`info`, `warn`, `error`)
- `limit` (1..500)

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
- `GUARDRAIL_BLOCKED`
- `GUARDRAIL_CONFIRM_REQUIRED`
- `RECOVERY_DISABLED`, `RECOVERY_ERROR`
