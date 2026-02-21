# Storage and Flush Operations

Sentinel stores runtime and feature state in SQLite (`sentinel.db` + WAL/SHM).

## Storage Stats

Endpoint:

- `GET /api/ops/storage/stats`

Returns:

- File size (`databaseBytes`, `walBytes`, `shmBytes`, `totalBytes`)
- Resource-level rows and approximate bytes
- Collection timestamp

Resources tracked:

- `timeline`
- `activity-journal`
- `guardrail-audit`
- `recovery-history`
- `ops-activity`
- `ops-alerts`
- `ops-jobs`

## Flush Resource Data

Endpoint:

- `POST /api/ops/storage/flush`

Payload:

```json
{ "resource": "all" }
```

Allowed values:

- `timeline`
- `activity-journal`
- `guardrail-audit`
- `recovery-history`
- `ops-activity`
- `ops-alerts`
- `ops-jobs`
- `all`

Response includes removed row counts per resource and flush timestamp.

## Operational Guidance

- Prefer targeted flush before full flush.
- Use full flush (`all`) for hard reset/testing environments.
- Flush triggers WAL checkpoint best-effort.

## UI Integration

Settings includes:

- Storage usage panel
- Per-resource flush actions
- Global flush action

Use this to keep timeline/history growth under control in long-running deployments.
