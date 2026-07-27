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

| Method   | Path              | Purpose           |
| -------- | ----------------- | ----------------- |
| `PUT`    | `/api/auth/token` | Set auth cookie   |
| `DELETE` | `/api/auth/token` | Clear auth cookie |

`PUT /api/auth/token` payload:

```json
{ "token": "..." }
```

## Metadata and Filesystem

| Method | Path           | Purpose                                                                                                                                                                     |
| ------ | -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/api/meta`    | Runtime metadata (`tokenRequired`, `defaultCwd`, `version`, `timezone`, `locale`, `hostname`, `processUser`, `isRoot`, `canSwitchUser`, `allowedUsers`, `userSwitchMethod`) |
| `GET`  | `/api/fs/dirs` | Directory suggestions for session creation                                                                                                                                  |

`/api/fs/dirs` query params: `prefix`, `limit`.

## Tmux Sessions

| Method   | Path                                | Purpose                                 |
| -------- | ----------------------------------- | --------------------------------------- |
| `GET`    | `/api/tmux/sessions`                | List sessions (enriched projection)     |
| `POST`   | `/api/tmux/sessions`                | Create session                          |
| `PATCH`  | `/api/tmux/sessions/{session}`      | Rename session                          |
| `PATCH`  | `/api/tmux/sessions/{session}/icon` | Set session icon                        |
| `DELETE` | `/api/tmux/sessions/{session}`      | Kill session                            |
| `PATCH`  | `/api/tmux/sessions/order`          | Reorder sessions                        |
| `POST`   | `/api/tmux/sessions/{session}/seen` | Mark seen scope (`pane/window/session`) |

Create payload:

```json
{ "name": "dev", "cwd": "/absolute/path", "icon": "rocket", "user": "deploy" }
```

`icon` and `user` are optional. On name collision the server tries `name-1` through `name-99`, so the response `name` may differ from the requested name.

## Window Launchers

| Method   | Path                                                       | Purpose                       |
| -------- | ---------------------------------------------------------- | ----------------------------- |
| `GET`    | `/api/tmux/launchers`                                      | List window launchers         |
| `POST`   | `/api/tmux/launchers`                                      | Create window launcher        |
| `PATCH`  | `/api/tmux/launchers/order`                                | Reorder window launchers      |
| `PATCH`  | `/api/tmux/launchers/{launcher}`                           | Update window launcher        |
| `DELETE` | `/api/tmux/launchers/{launcher}`                           | Delete window launcher        |
| `POST`   | `/api/tmux/sessions/{session}/launchers/{launcher}/launch` | Launch a window from launcher |

## Session Presets

Session presets back the `Pinned` sessions panel. Pinned sessions are saved and restored on Sentinel startup, and their endpoint keeps the pinned lifecycle separate from reusable session launchers.

| Method   | Path                                        | Purpose                      |
| -------- | ------------------------------------------- | ---------------------------- |
| `GET`    | `/api/tmux/session-presets`                 | List pinned session presets  |
| `POST`   | `/api/tmux/session-presets`                 | Pin a session preset         |
| `PATCH`  | `/api/tmux/session-presets/order`           | Reorder pinned sessions      |
| `PATCH`  | `/api/tmux/session-presets/{preset}`        | Update pinned session preset |
| `DELETE` | `/api/tmux/session-presets/{preset}`        | Unpin session preset         |
| `POST`   | `/api/tmux/session-presets/{preset}/launch` | Start or open pinned session |

## Session Launchers

Session launchers are independent reusable presets shown in the session `+` split-button menu and in `Manage session launchers...`. Creating or updating a launcher only saves the preset; it does not start a tmux session. Launching a session launcher creates a new session from the saved name seed, cwd, icon, and optional target user. If the base name already exists, Sentinel tries `name-1` through `name-99`.

| Method   | Path                                            | Purpose                      |
| -------- | ----------------------------------------------- | ---------------------------- |
| `GET`    | `/api/tmux/session-launchers`                   | List session launchers       |
| `POST`   | `/api/tmux/session-launchers`                   | Create session launcher      |
| `PATCH`  | `/api/tmux/session-launchers/order`             | Reorder session launchers    |
| `PATCH`  | `/api/tmux/session-launchers/{launcher}`        | Update session launcher      |
| `DELETE` | `/api/tmux/session-launchers/{launcher}`        | Delete session launcher      |
| `POST`   | `/api/tmux/session-launchers/{launcher}/launch` | Create session from launcher |

## Tmux Windows and Panes

| Method | Path                                         | Purpose       |
| ------ | -------------------------------------------- | ------------- |
| `GET`  | `/api/tmux/sessions/{session}/windows`       | List windows  |
| `GET`  | `/api/tmux/sessions/{session}/panes`         | List panes    |
| `POST` | `/api/tmux/sessions/{session}/select-window` | Select window |
| `POST` | `/api/tmux/sessions/{session}/select-pane`   | Select pane   |
| `POST` | `/api/tmux/sessions/{session}/new-window`    | Create window |
| `POST` | `/api/tmux/sessions/{session}/kill-window`   | Kill window   |
| `POST` | `/api/tmux/sessions/{session}/kill-pane`     | Kill pane     |
| `POST` | `/api/tmux/sessions/{session}/split-pane`    | Split pane    |
| `POST` | `/api/tmux/sessions/{session}/rename-window` | Rename window |
| `POST` | `/api/tmux/sessions/{session}/rename-pane`   | Rename pane   |

Split payload:

```json
{ "paneId": "%3", "direction": "vertical" }
```

Direction: `vertical` or `horizontal`.

## Tmux Activity

| Method | Path                       | Purpose                          |
| ------ | -------------------------- | -------------------------------- |
| `GET`  | `/api/tmux/activity/delta` | Delta patches by global revision |
| `GET`  | `/api/tmux/activity/stats` | Tmux activity runtime metrics    |
| `GET`  | `/api/tmux/frequent-dirs`  | Frequently used directories      |

`/api/tmux/activity/delta` query params:

- `since` (int64 >= 0)
- `limit` (1..1000)

`/api/tmux/frequent-dirs` query params:

- `limit` (1..20, default 5)

## Presence

| Method | Path                 | Purpose                                  |
| ------ | -------------------- | ---------------------------------------- |
| `PUT`  | `/api/tmux/presence` | Upsert terminal presence (HTTP fallback) |

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

## Now

| Method | Path                                          | Purpose                                      |
| ------ | --------------------------------------------- | -------------------------------------------- |
| `GET`  | `/api/now`                                    | Compose the current operational read model   |
| `POST` | `/api/now/services/{service}/runbook`         | Start the associated recovery procedure (202) |

`GET /api/now` fans out concurrently to Tmux, Services, Metrics, and Runbooks.
It returns `200` with every usable result when one source fails; only a missing
structural dependency returns `503 NOW_UNAVAILABLE`.

```json
{
  "now": {
    "generatedAt": "2026-07-27T12:00:00Z",
    "reliability": {
      "state": "normal",
      "services": {
        "tracked": 1,
        "running": 1,
        "failed": 0,
        "inactive": 0,
        "unknown": 0
      },
      "metrics": {
        "state": "normal",
        "severity": "ok",
        "warningCount": 0,
        "criticalCount": 0,
        "signals": []
      }
    },
    "attention": {
      "total": 0,
      "visible": [],
      "overflow": {
        "approvals": 0,
        "services": 0,
        "runbooks": 0,
        "metrics": 0
      }
    },
    "inProgress": {
      "runs": [],
      "sessions": []
    },
    "sources": {
      "tmux": { "status": "current", "checkedAt": "2026-07-27T12:00:00Z" },
      "services": { "status": "current", "checkedAt": "2026-07-27T12:00:00Z" },
      "metrics": { "status": "current", "checkedAt": "2026-07-27T12:00:00Z" },
      "runbooks": { "status": "current", "checkedAt": "2026-07-27T12:00:00Z" }
    }
  }
}
```

Source status is `current`, `stale`, `unavailable`, or `not_configured`.
Reliability is `degraded` whenever any source is not current; otherwise it is
`attention` for a failed tracked service or metrics pressure, and `normal`
when neither condition exists.

Attention items are discriminated by `type`: `runbook_approval`,
`service_failed`, `runbook_failed`, or `metrics_pressure`. The response keeps
at most five visible items in that priority order and reports hidden counts in
`overflow`. A runbook failure targeting the same failed service is returned as
`failure` evidence on the service item instead of as a duplicate item.

In-progress runs include only `queued` and `running` executions. Pending
approvals appear only in attention. In-progress Tmux data contains at most
three live pinned or unread sessions and never includes pane output, preview,
or command text.

The procedure action accepts the same optional parameter shape as the Runbooks
execution endpoint:

```json
{
  "parameters": {
    "MODE": "safe"
  }
}
```

The backend reloads the service and requires its current state to remain
`failed`, resolves its unique enabled `targetService` association, validates
parameters, and creates the job with `source: "now"` and the target copied from
the Runbook definition. A recovered service returns
`409 NOW_SERVICE_NOT_FAILED`; missing, disabled, or conflicting associations
return `NOW_RUNBOOK_NOT_FOUND`, `NOW_RUNBOOK_DISABLED`, or
`NOW_RUNBOOK_CONFLICT`. This endpoint never starts, stops, or restarts the
service directly.

## Operations: Control Plane

### Overview and Metrics

| Method   | Path                          | Purpose                            |
| -------- | ----------------------------- | ---------------------------------- |
| `GET`    | `/api/ops/overview`           | Host + Sentinel + services summary |
| `GET`    | `/api/ops/metrics`            | Host and Sentinel runtime metrics  |
| `GET`    | `/api/ops/config`             | Read config file                   |
| `PATCH`  | `/api/ops/config`             | Update config file                 |

`GET /api/ops/metrics` returns the raw sample and the canonical temporal
posture maintained by the shared Metrics evaluator:

```json
{
  "metrics": {
    "cpuPercent": 85,
    "memPercent": 65.2,
    "collectedAt": "2026-07-27T12:00:10Z"
  },
  "posture": {
    "state": "pressure",
    "severity": "warning",
    "warningCount": 1,
    "criticalCount": 0,
    "signals": [
      {
        "name": "cpu",
        "severity": "warning",
        "value": 85,
        "since": "2026-07-27T12:00:00Z"
      }
    ],
    "observedAt": "2026-07-27T12:00:10Z"
  }
}
```

Posture state is `normal`, `pressure`, or `unavailable`; severity is `ok`,
`warning`, `critical`, or `unknown`. `signals` contains only warning/critical
entries, each with its canonical `name`, `severity`, sampled `value`, and
threshold-crossing `since`. `observedAt` records when the shared evaluator last
classified the posture. Arrays are always present, including `signals: []` for
normal and unavailable states.

Capacity signals (`rootDisk`, `inodes`) and PSI averages enter immediately.
CPU, memory, and swap require ten continuous seconds above a threshold, as does
their warning-to-critical escalation. Exit thresholds use the hysteresis
documented in [Metrics](../features/metrics.md).

### Services

| Method   | Path                                 | Purpose                                   |
| -------- | ------------------------------------ | ----------------------------------------- |
| `GET`    | `/api/ops/services`                  | Tracked service list and runtime status   |
| `GET`    | `/api/ops/services/browse`           | Browse all host units with tracked status |
| `GET`    | `/api/ops/services/discover`         | Discover available services               |
| `POST`   | `/api/ops/services`                  | Register custom service                   |
| `DELETE` | `/api/ops/services/{service}`        | Unregister custom service                 |
| `POST`   | `/api/ops/services/{service}/action` | Execute a service action and verify it    |
| `GET`    | `/api/ops/services/{service}/status` | Detailed manager status for one service   |
| `GET`    | `/api/ops/services/{service}/logs`   | Service logs                              |
| `POST`   | `/api/ops/services/unit/action`      | Act on unit directly by name              |
| `GET`    | `/api/ops/services/unit/status`      | Inspect unit directly                     |
| `GET`    | `/api/ops/services/unit/logs`        | Unit logs directly                        |

Tracked service and Browse entries expose `trackingMode` as `builtin` or
`custom`. Built-ins are resolved from canonical systemd units or launchd labels
at request time and are not persisted in `ops_custom_services`. Registering a
reserved built-in name/unit or deleting a built-in identity returns
`409 OPS_SERVICE_BUILTIN`.

Service action payload:

```json
{ "action": "restart" }
```

Actions accept `start`, `stop`, `restart`, `enable`, or `disable`. An accepted
command returns HTTP `200` with bounded post-condition evidence:

```json
{
  "verification": {
    "state": "confirmed",
    "field": "activeState",
    "expected": "active",
    "observed": "active",
    "observedAt": "2026-07-27T15:30:01Z",
    "attempts": 2
  }
}
```

`state` is `confirmed`, `mismatch`, or `unavailable`. `mismatch` and
`unavailable` still return HTTP `200` because the manager accepted the command;
only `confirmed` proves its post-condition. Start/restart expect
`activeState=active`, stop expects `activeState=inactive`, enable expects
`enabledState=enabled`, and disable expects `enabledState=disabled`.

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

Status responses expose `status.observedAt` and a structured
`status.condition` with manager-supported values among `activeState`,
`subState`, `result`, `exitCode`, `exitStatus`, and `transitionedAt`. Tracked
status also returns `context.runbook` and `context.latestRun`; absent values are
JSON `null`.

Unit query params (status and logs): `unit`, `scope`, `manager`, `lines`.
Tracked and unit log endpoints also accept optional `since=<RFC3339>`. Invalid
timestamps return `400 INVALID_REQUEST`.

### Runbooks

| Method   | Path                              | Purpose                               |
| -------- | --------------------------------- | ------------------------------------- |
| `GET`    | `/api/ops/runbooks`               | List runbooks and recent jobs         |
| `POST`   | `/api/ops/runbooks`               | Create custom runbook                 |
| `PUT`    | `/api/ops/runbooks/{runbook}`     | Update runbook                        |
| `DELETE` | `/api/ops/runbooks/{runbook}`     | Delete runbook                        |
| `POST`   | `/api/ops/runbooks/{runbook}/run` | Execute runbook asynchronously (202)  |
| `GET`    | `/api/ops/jobs/{job}`             | Query one runbook job                 |
| `DELETE` | `/api/ops/jobs/{job}`             | Delete a runbook job                  |
| `POST`   | `/api/ops/runs/{runId}/approve`   | Approve a waiting approval step (202) |
| `POST`   | `/api/ops/runs/{runId}/reject`    | Reject a waiting approval step        |

Runbook create/update payload:

```json
{
  "name": "Health Check",
  "description": "Verify service health",
  "enabled": true,
  "webhookURL": "https://hooks.example.com/sentinel",
  "targetService": "myapp",
  "steps": [
    { "type": "run", "title": "Check status", "command": "systemctl --user is-active myapp" },
    {
      "type": "run",
      "title": "Verify response",
      "command": "curl -sf http://localhost:8080/health"
    },
    {
      "type": "script",
      "title": "Rotate logs",
      "script": "#!/bin/bash\ncd /var/log && gzip *.log"
    },
    { "type": "approval", "title": "Review", "description": "Inspect output above." }
  ]
}
```

Step types:

- `run` — execute a single shell command (`command` field).
- `script` — execute a multi-line script (`script` field).
- `approval` — pause and wait for manual approval (`description` field).

Per-step options (all optional):

| Field             | Type | Description                          |
| ----------------- | ---- | ------------------------------------ |
| `continueOnError` | bool | Continue to the next step on failure |
| `timeout`         | int  | Step timeout in seconds              |
| `retries`         | int  | Number of retry attempts             |
| `retryDelay`      | int  | Delay between retries in seconds     |

The optional `webhookURL` field configures a webhook endpoint that receives a POST with run results on completion. Must be `http` or `https`. See [Runbooks — Webhooks](/features/runbooks.md#webhooks) for payload details.

`targetService` is optional, must be present in the tracked Services catalog,
and is unique across Runbooks. Conflicts return
`409 OPS_RUNBOOK_TARGET_CONFLICT`. Deleting an associated custom service returns
`409 OPS_SERVICE_IN_USE`.

New job objects include `source` (`runbooks`, `scheduler`, or `now`). Jobs
derived from an associated definition also include `targetKind: "service"` and
`targetName`. These fields are omitted from historical jobs where their stored
values are empty. New jobs include a versioned `definition` receipt containing
the exact runbook metadata, steps, parameter definitions, webhook, and target
used by the execution. `parametersUsed` values are persisted in clear text and
visible to operators; they must not contain secrets.

A service target can have only one queued, running, or waiting-for-approval
execution. Competing starts return `409 RUNBOOK_TARGET_BUSY`. Sentinel installs
no default runbooks; procedures must be authored explicitly for the host.

### Schedules

| Method   | Path                                    | Purpose                      |
| -------- | --------------------------------------- | ---------------------------- |
| `GET`    | `/api/ops/schedules`                    | List schedules               |
| `POST`   | `/api/ops/schedules`                    | Create schedule              |
| `PUT`    | `/api/ops/schedules/{schedule}`         | Update schedule              |
| `DELETE` | `/api/ops/schedules/{schedule}`         | Delete schedule              |
| `POST`   | `/api/ops/schedules/{schedule}/trigger` | Trigger schedule immediately |

### Settings and Config

| Method  | Path                         | Purpose                         |
| ------- | ---------------------------- | ------------------------------- |
| `GET`   | `/api/ops/config`            | Get redacted effective config   |
| `PATCH` | `/api/ops/config`            | Update editable config sections |
| `PATCH` | `/api/ops/settings/timezone` | Update timezone                 |
| `PATCH` | `/api/ops/settings/locale`   | Update locale                   |
| `GET`   | `/api/ops/settings/mcp`      | Read live MCP availability      |
| `PATCH` | `/api/ops/settings/mcp`      | Enable or disable `/mcp` live   |

## Operations: Storage

| Method | Path                     | Purpose                   |
| ------ | ------------------------ | ------------------------- |
| `GET`  | `/api/ops/storage/stats` | Storage usage by resource |
| `POST` | `/api/ops/storage/flush` | Flush resource data       |

Flush payload:

```json
{ "resource": "ops-jobs" }
```

Allowed resources:

- `activity-journal`
- `ops-jobs`
- `all`

## Common Error Codes

- `INVALID_REQUEST`
- `UNAUTHORIZED`
- `ORIGIN_DENIED`
- `STORE_ERROR`
- `OPS_RUNBOOK_TARGET_NOT_FOUND`
- `OPS_RUNBOOK_TARGET_CONFLICT`
- `RUNBOOK_TARGET_BUSY` — 409 — Service target already has an active execution
- `OPS_SERVICE_IN_USE`
- `UNAVAILABLE`
- `TMUX_*` (`TMUX_NOT_FOUND`, `SESSION_NOT_FOUND`, etc.)
- `OPS_RUNBOOK_NOT_FOUND`, `OPS_JOB_NOT_FOUND`
- `SCHEDULE_NOT_FOUND`
- `USER_NOT_ALLOWED` — 403 — Target user not in allowlist or system users
- `TMUX_LAUNCHER_NOT_FOUND` — 404 — Referenced launcher does not exist
- `TMUX_LAUNCHER_EXISTS` — 409 — Launcher with this name already exists
- `INVALID_STATE` — 409 — Operation not valid in the current state (e.g., runbook step approve/reject)
