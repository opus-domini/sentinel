# Storage and Flush Operations

Sentinel stores runtime and feature state in SQLite (`sentinel.db` + WAL/SHM).

CLI helpers:

```bash
sentinel db init
sentinel db status
sentinel db reset --yes
sentinel db reset --yes --force
```

## Storage Stats

Endpoint:

- `GET /api/ops/storage/stats`

Returns:

- File size (`databaseBytes`, `walBytes`, `shmBytes`, `totalBytes`)
- Resource-level `totalRows`, `flushableRows`, `protectedRows`, and approximate
  bytes
- Collection timestamp

Resources tracked:

- `activity-journal`
- `ops-jobs`

## Flush Resource Data

Endpoint:

- `POST /api/ops/storage/flush`

Payload:

```json
{ "resource": "all" }
```

Allowed values:

- `activity-journal`
- `ops-jobs`
- `all`

Response includes removed row counts per resource and flush timestamp.

Every flush runs inside one SQLite transaction. `all` clears both resources in
that same transaction; if either resource fails, neither resource is changed.
The response also includes `protectedRows` so the caller can confirm how many
rows were deliberately preserved.

For `ops-jobs`, only terminal executions are eligible:

- `succeeded`
- `failed`

Queued, running, and approval-waiting executions are protected. Unknown or
future non-terminal states are protected as well; cleanup never uses a blanket
delete against the jobs table.

## Operational Guidance

- Prefer targeted flush before full flush.
- Use full flush (`all`) when all eligible historical rows should be removed
  atomically.
- Use `sentinel db reset --yes --force` when the intended operation is a full
  SQLite wipe and migration replay.
- Flush triggers WAL checkpoint best-effort.

## UI Integration

Settings links to `/maintenance/storage`, a dedicated workspace with:

- storage footprint and resource counts;
- explicit eligible/protected impact;
- resource selection;
- destructive confirmation before cleanup;
- success and failure receipts.

The Settings dialog itself does not execute destructive storage actions.
