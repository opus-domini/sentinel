# Runbooks

![Desktop runbooks](assets/images/desktop-runbooks.png)

Runbooks are executable operational procedures — sequences of steps that run against the host. Each execution is tracked as a job with step-level output persistence.

## Step Types

Each runbook contains an ordered list of steps. Three types are supported:

- **command** — runs a shell command via `sh -c`, captures combined stdout+stderr
- **check** — runs a shell command as a validation/assertion step
- **manual** — informational step with a description, no execution

Steps execute sequentially. The first `command` or `check` failure stops the run.

## Built-in Runbooks

Sentinel seeds two runbooks on first startup:

**Service Recovery** (`ops.service.recover`)

1. `command` — Inspect service status (`sentinel service status`)
2. `command` — Restart service (`sentinel service install --start=true`)
3. `check` — Confirm healthy status

**Autoupdate Verification** (`ops.autoupdate.verify`)

1. `command` — Check updater timer (`sentinel service autoupdate status`)
2. `command` — Check release status (`sentinel update check`)
3. `manual` — Review versions and update policy before apply

Built-in runbooks (IDs prefixed with `ops.`) cannot be deleted.

## Custom Runbooks

Create custom runbooks via the API or the frontend editor.

**Create:**

```
POST /api/ops/runbooks
```

```json
{
  "name": "My Runbook",
  "description": "Optional description",
  "enabled": true,
  "steps": [
    { "type": "command", "title": "List files", "command": "ls -la /tmp" },
    { "type": "check", "title": "Verify disk", "check": "df -h / | grep -v 100%" },
    { "type": "manual", "title": "Review", "description": "Check output above." }
  ]
}
```

Returns `201` with `{ runbook }`.

**Update:**

```
PUT /api/ops/runbooks/{runbook}
```

Same payload shape. Returns `200` with `{ runbook }`.

**Delete:**

```
DELETE /api/ops/runbooks/{runbook}
```

Returns `200` with `{ removed: "<id>" }`. Fails with `404` for unknown IDs.

## Execution

Trigger a run:

```
POST /api/ops/runbooks/{runbook}/run
```

Returns `202` with the initial job object. Execution runs asynchronously in a background goroutine with a 5-minute overall timeout and 30-second per-step timeout.

Job status lifecycle: `queued` -> `running` -> `succeeded` | `failed`

At each step completion, the job is updated in the store and an `ops.job.updated` event is emitted with the full job object including accumulated step results.

Timeline events are created at runbook start (`runbook.started`) and completion (`runbook.succeeded` or `runbook.failed`).

## Webhooks

Runbooks can optionally define a `webhookURL` field to receive HTTP notifications when a run completes. Set the URL via the editor UI or the create/update API. An empty string disables the webhook.

URL validation requires `http` or `https` scheme with a valid host.

When a run finishes (succeeded or failed), Sentinel sends a `POST` request to the configured URL with a JSON payload. Delivery uses a 10-second timeout with exponential backoff retry (3 attempts) on 5xx responses. Webhooks fire for both manual and scheduled runs.

**Payload:**

```json
{
  "event": "runbook.completed",
  "sentAt": "2026-02-20T22:01:00Z",
  "runbook": {
    "id": "rb-7",
    "name": "Deploy Service"
  },
  "job": {
    "id": "run-42",
    "status": "succeeded",
    "source": "scheduler",
    "totalSteps": 3,
    "completedSteps": 3,
    "startedAt": "2026-02-20T22:00:00Z",
    "finishedAt": "2026-02-20T22:01:00Z",
    "steps": [
      { "index": 0, "title": "Build", "type": "command", "output": "ok", "durationMs": 120 },
      { "index": 1, "title": "Test", "type": "command", "output": "passed", "durationMs": 340 },
      { "index": 2, "title": "Verify", "type": "check", "durationMs": 50 }
    ]
  }
}
```

Fields use `omitempty` — `error`, `startedAt`, `finishedAt`, and step-level `output` are omitted when empty. On a failed run, `error` appears at the job level and optionally on the failing step:

```json
{
  "event": "runbook.completed",
  "sentAt": "2026-02-20T22:05:00Z",
  "runbook": {
    "id": "rb-7",
    "name": "Deploy Service"
  },
  "job": {
    "id": "run-43",
    "status": "failed",
    "source": "manual",
    "error": "step 1 failed: exit status 1",
    "totalSteps": 3,
    "completedSteps": 1,
    "startedAt": "2026-02-20T22:04:00Z",
    "finishedAt": "2026-02-20T22:05:00Z",
    "steps": [
      { "index": 0, "title": "Build", "type": "command", "output": "ok", "durationMs": 120 },
      { "index": 1, "title": "Test", "type": "command", "error": "exit status 1", "durationMs": 410 }
    ]
  }
}
```

## Step Results

Each step result includes:

- `stepIndex` — zero-based position
- `title` — step title
- `type` — `command`, `check`, or `manual`
- `output` — captured stdout+stderr (or description for manual steps)
- `error` — error message if the step failed
- `durationMs` — execution time in milliseconds

Results are persisted as JSON in the `step_results` column of `ops_runbook_runs` and included in every job object returned by the API and WebSocket events.

## Job History

Jobs are listed alongside runbooks in the list response:

```
GET /api/ops/runbooks
```

Returns `{ runbooks, jobs }` where `jobs` contains the 20 most recent runs.

Query a single job:

```
GET /api/ops/jobs/{job}
```

Delete a job:

```
DELETE /api/ops/jobs/{job}
```

## Realtime Events

- `ops.job.updated` — emitted on each state change (queued, running, per-step progress, completion)
- Each event payload includes `{ globalRev, job }` with the full job object and accumulated `stepResults`
- `ops.timeline.updated` — emitted for runbook start and completion timeline entries

## Frontend

The dedicated `/runbooks` route provides a standalone page for runbook execution and job history:

- Sidebar listing all runbooks with run counts
- Detail view showing step overview and a run button
- Job history cards, expandable to reveal per-step results
- Each step result is collapsible with output or "No output" indicator
- Job deletion with inline confirmation
- Editor for creating and editing custom runbooks with drag-to-reorder steps

## API Endpoints

- `GET /api/ops/runbooks` — list runbooks and recent jobs
- `POST /api/ops/runbooks` — create custom runbook
- `PUT /api/ops/runbooks/{runbook}` — update runbook
- `DELETE /api/ops/runbooks/{runbook}` — delete runbook
- `POST /api/ops/runbooks/{runbook}/run` — trigger execution
- `GET /api/ops/jobs/{job}` — get job details
- `DELETE /api/ops/jobs/{job}` — delete job
