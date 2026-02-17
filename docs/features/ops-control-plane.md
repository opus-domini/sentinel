# Ops Control Plane

The `/ops` route centralizes host operations in a single event-driven panel.

## What You Get

- **Overview**: host identity, Sentinel uptime, service health summary.
- **[Services](/features/services.md)**: monitor, start/stop/restart, browse and register host services.
- **[Runbooks](/features/runbooks.md)**: executable operational procedures with step-level output tracking.
- **Alerts**: deduplicated alert feed with acknowledge action.
- **Timeline**: searchable operational timeline by severity/text.

## Realtime Model

- Initial state loads from HTTP API.
- Continuous updates come from `/ws/events`.
- Primary events:
  - `ops.overview.updated`
  - `ops.services.updated`
  - `ops.alerts.updated`
  - `ops.timeline.updated`
  - `ops.job.updated`

## API Surface

Overview and alerts:

- `GET /api/ops/overview`
- `GET /api/ops/alerts`
- `POST /api/ops/alerts/{alert}/ack`
- `GET /api/ops/timeline`
- `GET /api/ops/metrics`
- `GET /api/ops/config`
- `PATCH /api/ops/config`

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

## UX Notes

- Service actions are optimistic and reconcile with API/events.
- Alert ack is optimistic with rollback on failure.
- Timeline prepends new events without full list refetch.
- Runbook jobs are updated in place as events arrive via WebSocket.
