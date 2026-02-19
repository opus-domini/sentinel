# Recovery Engine

Recovery persists tmux session state so workspaces can be restored after crashes, reboot, or tmux server loss.

## What Recovery Captures

Per session snapshot:

- Windows and panes layout
- Active window and pane
- Commands/path metadata
- Tail content preview
- Session state hash for deduplication

Snapshots are stored in SQLite and versioned over time.

## Runtime Behavior

- Periodic collect (default `5s`).
- Tracks host boot ID and live session set.
- Marks tracked sessions as `killed` when boot changes and sessions disappear.
- Emits realtime events:
  - `recovery.overview.updated`
  - `recovery.job.updated`

## Restore Modes

`POST /api/recovery/snapshots/{id}/restore` supports:

- `mode`: `safe`, `confirm`, `full`
- `conflictPolicy`: `rename`, `replace`, `skip`
- `targetSession`: optional session name override

Result is asynchronous job execution with progress.

## CLI Workflows

List sessions by state:

```bash
sentinel recovery list --state killed,restored --limit 100
```

Restore snapshot and wait:

```bash
sentinel recovery restore --snapshot 42 --mode confirm --conflict rename --wait=true
```

## API Endpoints

- `GET /api/recovery/overview`
- `GET /api/recovery/sessions`
- `POST /api/recovery/sessions/{session}/archive`
- `GET /api/recovery/sessions/{session}/snapshots`
- `GET /api/recovery/snapshots/{snapshot}`
- `POST /api/recovery/snapshots/{snapshot}/restore`
- `GET /api/recovery/jobs/{job}`
