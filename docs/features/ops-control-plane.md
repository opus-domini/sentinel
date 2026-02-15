# Ops Control Plane

The `/ops` route centralizes host operations in a single event-driven panel.

## What You Get

- **Overview**: host identity, Sentinel uptime, service health summary.
- **Services**: start/stop/restart with optimistic UI updates.
- **Service status inspect**: open manager details (`systemctl`/`launchctl`) per service.
- **Alerts**: deduplicated alert feed with acknowledge action.
- **Timeline**: searchable operational timeline by severity/text.
- **Runbooks**: executable operational procedures with tracked job status.

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

- `GET /api/ops/overview`
- `GET /api/ops/services`
- `POST /api/ops/services/{service}/action`
- `GET /api/ops/alerts`
- `POST /api/ops/alerts/{alert}/ack`
- `GET /api/ops/timeline`
- `GET /api/ops/runbooks`
- `POST /api/ops/runbooks/{runbook}/run`
- `GET /api/ops/jobs/{job}`

## UX Notes

- Service actions are optimistic and reconcile with API/events.
- Alert ack is optimistic with rollback on failure.
- Timeline prepends new events without full list refetch.
- Runbook jobs are updated in place as events arrive.
