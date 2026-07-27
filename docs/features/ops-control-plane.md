# Ops Control Plane

![Desktop Now](assets/images/desktop-now.png)

The ops control plane is Sentinel's host operations management layer. Now
connects the current evidence and hands each decision to its dedicated owner
page.

## Pages

| Route       | Feature            | Description                                                                        | Documentation                     |
| ----------- | ------------------ | ---------------------------------------------------------------------------------- | --------------------------------- |
| `/`         | Now                | Host posture, evidence confidence, operator attention, and owner-module handoffs   | [Now](/features/now.md)            |
| `/tmux`     | Tmux Workspace     | Existing sessions, terminals, windows, panes, unread state, and launchers           | [Tmux](/features/tmux-workspace.md) |
| `/runbooks` | Runbook Execution  | Executable operational procedures with step-level output tracking and job history  | [Runbooks](/features/runbooks.md)  |
| `/services` | Service Management | Monitor, start/stop/restart, browse, and register systemd/launchd services          | [Services](/features/services.md)  |
| `/metrics`  | System Metrics     | Canonical host posture plus detailed system and runtime metrics                     | [Metrics](/features/metrics.md)    |

## Shared Infrastructure

### Realtime Model

- Initial state loads from HTTP API.
- Continuous updates come from `/ws/events`.
- Now composes the existing owner resources and refreshes from semantic owner
  events; it does not add a polling loop or a new event.
- Primary events:
  - `ops.overview.updated`
  - `ops.services.updated`
  - `ops.job.updated`
  - `ops.runbooks.updated`
  - `ops.schedule.updated`
  - `ops.metrics.updated`
  - `ops.posture.updated`

Freshness remains owned by each module. Services watches its canonical state
every five seconds and emits only on change. Metrics publishes every sample for
its owner page and emits a separate posture event only when the semantic signal
set changes. Runbooks publishes definition changes from the shared HTTP/MCP
manager. Now recomposes from those events instead of duplicating collectors.

### API Surface

Overview and configuration:

- `GET /api/now`
- `POST /api/now/services/{service}/runbook`
- `GET /api/ops/overview`
- `GET /api/ops/config`
- `PATCH /api/ops/config`

Metrics (see [Metrics](/features/metrics.md)):

- `GET /api/ops/metrics`

Services (see [Services](/features/services.md)):

- `GET /api/ops/services`
- `GET /api/ops/services/browse`
- `GET /api/ops/services/discover`
- `POST /api/ops/services`
- `DELETE /api/ops/services/{service}`
- `POST /api/ops/services/{service}/action`
- `GET /api/ops/services/{service}/status`
- `GET /api/ops/services/{service}/logs`
- `POST /api/ops/services/unit/action`
- `GET /api/ops/services/unit/status`
- `GET /api/ops/services/unit/logs`

Runbooks (see [Runbooks](/features/runbooks.md)):

- `GET /api/ops/runbooks`
- `POST /api/ops/runbooks`
- `PUT /api/ops/runbooks/{runbook}`
- `DELETE /api/ops/runbooks/{runbook}`
- `POST /api/ops/runbooks/{runbook}/run`
- `GET /api/ops/jobs/{job}`
- `DELETE /api/ops/jobs/{job}`
- `POST /api/ops/runs/{runId}/approve`
- `POST /api/ops/runs/{runId}/reject`

Schedules (see [Runbooks](/features/runbooks.md)):

- `GET /api/ops/schedules`
- `POST /api/ops/schedules`
- `PUT /api/ops/schedules/{schedule}`
- `DELETE /api/ops/schedules/{schedule}`
- `POST /api/ops/schedules/{schedule}/trigger`

## Navigation

Use `/` as the operational starting point. Follow its links into `/tmux`,
`/runbooks`, `/services`, or `/metrics` for the owning workflow.
