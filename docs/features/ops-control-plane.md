# Ops Control Plane

![Desktop alerts](assets/images/desktop-alerts.png)

The ops control plane is Sentinel's host operations management layer. It provides visibility into system services, operational procedures, alerts, activity timelines, and runtime metrics through a set of dedicated pages accessible from the side rail.

## Pages

| Route | Feature | Description | Documentation |
|---|---|---|---|
| `/services` | Service Management | Monitor, start/stop/restart, browse and register systemd/launchd services | [Services](/features/services.md) |
| `/runbooks` | Runbook Execution | Executable operational procedures with step-level output tracking and job history | [Runbooks](/features/runbooks.md) |
| `/alerts` | Alert Monitoring | Deduplicated alert feed with acknowledge action | [Alerts](/features/alerts.md) |
| `/timeline` | Operations Timeline | Searchable operational audit log filtered by severity and text | [Timeline and Watchtower](/features/timeline-watchtower.md) |
| `/metrics` | System Metrics | System and runtime metrics dashboard | [Metrics](/features/metrics.md) |

## Shared Infrastructure

### Realtime Model

- Initial state loads from HTTP API.
- Continuous updates come from `/ws/events`.
- Primary events:
  - `ops.overview.updated`
  - `ops.services.updated`
  - `ops.alerts.updated`
  - `ops.timeline.updated`
  - `ops.job.updated`

### API Surface

Overview and configuration:

- `GET /api/ops/overview`
- `GET /api/ops/config`
- `PATCH /api/ops/config`

Alerts (see [Alerts](/features/alerts.md)):

- `GET /api/ops/alerts`
- `POST /api/ops/alerts/{alert}/ack`

Timeline (see [Timeline and Watchtower](/features/timeline-watchtower.md)):

- `GET /api/ops/timeline`

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

## Navigation

The `/ops` path redirects to `/alerts` for backward compatibility.
