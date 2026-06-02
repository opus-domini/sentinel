# Ops Control Plane

![Desktop services](assets/images/desktop-services.png)

The ops control plane is Sentinel's host operations management layer. It provides visibility into system services, operational procedures, and runtime metrics through dedicated pages accessible from the side rail.

## Pages

| Route         | Feature             | Description                                                                       | Documentation                                               |
| ------------- | ------------------- | --------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| `/services`   | Service Management  | Monitor, start/stop/restart, browse and register systemd/launchd services         | [Services](/features/services.md)                           |
| `/runbooks`   | Runbook Execution   | Executable operational procedures with step-level output tracking and job history | [Runbooks](/features/runbooks.md)                           |
| `/metrics`    | System Metrics      | System and runtime metrics dashboard                                              | [Metrics](/features/metrics.md)                             |

## Shared Infrastructure

### Realtime Model

- Initial state loads from HTTP API.
- Continuous updates come from `/ws/events`.
- Primary events:
  - `ops.overview.updated`
  - `ops.services.updated`
  - `ops.job.updated`
  - `ops.schedule.updated`
  - `ops.metrics.updated`

### API Surface

Overview and configuration:

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

Use `/services`, `/metrics`, and `/runbooks` for the active operations workflows.
