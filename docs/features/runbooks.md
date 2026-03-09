# Runbooks

![Desktop runbooks](assets/images/desktop-runbooks.png)

Runbooks are executable operational procedures — sequences of steps that run against the host. Each execution is tracked as a job with step-level output persistence.

## Step Types

Each runbook contains an ordered list of steps. Three types are supported:

- **run** — runs a single shell command via `sh -c`, captures combined stdout+stderr
- **script** — writes a multiline script to a temporary file and executes it with shebang support (e.g. `#!/usr/bin/env bash`)
- **approval** — pauses execution and waits for a human to approve or reject via the API before continuing

Steps execute sequentially. The first `run` or `script` failure stops the run (unless `continueOnError` is set on the step).

### Per-step Options

Each step supports optional fields that control execution behavior:

- `continueOnError` (bool) — when `true`, a step failure does not stop the run
- `timeout` (int, seconds) — per-step timeout override; defaults to 30 seconds
- `retries` (int) — number of retry attempts on failure; approval steps are never retried
- `retryDelay` (int, seconds) — delay between retries; defaults to 2 seconds

## Built-in Runbooks

Sentinel seeds three runbooks on first startup:

**Service Recovery** (`ops.service.recover`)

1. `run` — Inspect service status (`sentinel service status`)
2. `run` — Restart service (`sentinel service install --start=true`)
3. `run` — Confirm healthy status

**Autoupdate Verification** (`ops.autoupdate.verify`)

1. `run` — Check updater timer (`sentinel service autoupdate status`)
2. `run` — Check release status (`sentinel update check`)
3. `approval` — Review versions and update policy before apply

**Apply Update** (`ops.update.apply`)

1. `run` — Check for updates (`sentinel update check`)
2. `run` — Apply update and restart (`sentinel update apply --restart`)

Built-in runbooks (IDs prefixed with `ops.`) cannot be deleted.

## Parameters

Runbooks can define a `parameters` array. Each parameter has:

- `name` — identifier used in `{{NAME}}` placeholders
- `label` — human-readable label
- `type` — `string`, `number`, `boolean`, or `select`
- `default` — default value (used when the caller omits the parameter)
- `required` — when `true`, the run fails validation if the value is empty
- `options` — list of allowed values (for `select` type only)

When a run is triggered, supplied parameter values are merged with defaults. `{{PARAM}}` placeholders in step commands and scripts are replaced with shell-escaped values before execution. The resolved parameter map is persisted in the `parametersUsed` field of the run record.

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
  "parameters": [
    { "name": "SERVICE", "label": "Service name", "type": "string", "required": true }
  ],
  "steps": [
    { "type": "run", "title": "Check service", "command": "systemctl status {{SERVICE}}" },
    { "type": "script", "title": "Gather logs", "script": "#!/usr/bin/env bash\njournalctl -u {{SERVICE}} --no-pager -n 50" },
    { "type": "approval", "title": "Confirm restart", "description": "Review output before restarting." },
    { "type": "run", "title": "Restart", "command": "systemctl restart {{SERVICE}}", "continueOnError": true, "timeout": 60, "retries": 2, "retryDelay": 5 }
  ]
}
```

Returns `201` with `{ runbook }`. The response may also include a `shellWarnings` array if any `run` or `script` steps contain shell syntax issues (validated via `mvdan.cc/sh`). Warnings are non-blocking — the runbook is still saved.

**Update:**

```
PUT /api/ops/runbooks/{runbook}
```

Same payload shape. Returns `200` with `{ runbook }` (plus optional `shellWarnings`).

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

Optional request body for parameterized runbooks:

```json
{
  "parameters": {
    "SERVICE": "nginx"
  }
}
```

Returns `202` with the initial job object. Execution runs asynchronously in a background goroutine with a 5-minute overall timeout and 30-second per-step timeout (overridable per step).

Job status lifecycle: `queued` -> `running` -> `succeeded` | `failed` | `waiting_approval`

When an `approval` step is reached, the run transitions to `waiting_approval` and pauses. Use the approve/reject endpoints to continue or abort:

```
POST /api/ops/runs/{runId}/approve
```

Resumes execution from the step after the approval step. Returns `202`.

```
POST /api/ops/runs/{runId}/reject
```

Marks the run as `failed` with error "approval rejected". Returns `200`.

Both endpoints return `409 INVALID_STATE` if the run is not in `waiting_approval` status.

At each step completion, the job is updated in the store and an `ops.job.updated` event is emitted with the full job object including accumulated step results.

Timeline events are created at runbook start (`runbook.started`) and completion (`runbook.succeeded` or `runbook.failed`).

## Shell Validation

On create and update, Sentinel validates shell syntax for all `run` and `script` steps using `mvdan.cc/sh`. Warnings are returned in the response as a `shellWarnings` array:

```json
{
  "runbook": { "..." : "..." },
  "shellWarnings": [
    { "step": 0, "line": 1, "column": 12, "message": "unexpected token" }
  ]
}
```

Shell warnings are advisory — they do not block saving the runbook.

## Runbook Suggestions

```
GET /api/ops/runbooks/suggest?marker={marker}&session={session}
```

Returns up to 5 enabled runbooks whose name or description matches the given marker or session name. Results are ranked by relevance (name matches above description-only matches). Useful for suggesting relevant runbooks based on alert context.

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
      { "index": 0, "title": "Build", "type": "run", "output": "ok", "durationMs": 120 },
      { "index": 1, "title": "Test", "type": "script", "output": "passed", "durationMs": 340 },
      { "index": 2, "title": "Verify", "type": "run", "durationMs": 50 }
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
    "source": "runbook",
    "error": "step 1 failed: exit status 1",
    "totalSteps": 3,
    "completedSteps": 1,
    "startedAt": "2026-02-20T22:04:00Z",
    "finishedAt": "2026-02-20T22:05:00Z",
    "steps": [
      { "index": 0, "title": "Build", "type": "run", "output": "ok", "durationMs": 120 },
      { "index": 1, "title": "Test", "type": "run", "error": "exit status 1", "durationMs": 410 }
    ]
  }
}
```

## Step Results

Each step result includes:

- `stepIndex` — zero-based position
- `title` — step title
- `type` — `run`, `script`, or `approval`
- `output` — captured stdout+stderr (or description for approval steps)
- `error` — error message if the step failed
- `durationMs` — execution time in milliseconds

Results are persisted as JSON in the `step_results` column of `ops_runbook_runs` and included in every job object returned by the API and WebSocket events.

## Job History

Jobs are listed alongside runbooks in the list response:

```
GET /api/ops/runbooks
```

Returns `{ runbooks, jobs, schedules }` where `jobs` contains the 20 most recent runs.

Query a single job:

```
GET /api/ops/jobs/{job}
```

Delete a job:

```
DELETE /api/ops/jobs/{job}
```

## Scheduling

Runbooks can be executed on a schedule. Two schedule types are supported:

- **Cron** — recurring execution using standard cron expressions (e.g. `0 */6 * * *`). Supports optional timezone via IANA identifiers (e.g. `America/New_York`); defaults to the host's local timezone.
- **One-shot** — single future execution at a specific time, automatically removed after firing.

Schedules are managed via the API and the frontend editor. A background scheduler engine evaluates pending schedules every minute and triggers runs as they come due. Scheduled runs use `"source": "scheduler"` in job objects and webhook payloads.

When a schedule is created, updated, or deleted, an `ops.schedule.updated` event is emitted over the `/ws/events` WebSocket.

## Realtime Events

- `ops.job.updated` — emitted on each state change (queued, running, per-step progress, waiting_approval, completion)
- Each event payload includes `{ globalRev, job }` with the full job object and accumulated `stepResults`
- `ops.activity.updated` — emitted for runbook start and completion timeline entries
- `ops.schedule.updated` — emitted when a schedule is created, modified, or removed

## Frontend

The dedicated `/runbooks` route provides a standalone page for runbook execution and job history:

- Sidebar listing all runbooks with run counts
- Detail view showing step overview and a run button
- Job history cards, expandable to reveal per-step results
- Each step result is collapsible with output or "No output" indicator
- Job deletion with inline confirmation
- Editor for creating and editing custom runbooks with drag-to-reorder steps
- Schedule management: create, edit, and delete cron or one-shot schedules per runbook

## API Endpoints

- `GET /api/ops/runbooks` — list runbooks and recent jobs
- `GET /api/ops/runbooks/suggest` — suggest runbooks for a marker/session
- `POST /api/ops/runbooks` — create custom runbook
- `PUT /api/ops/runbooks/{runbook}` — update runbook
- `DELETE /api/ops/runbooks/{runbook}` — delete runbook
- `POST /api/ops/runbooks/{runbook}/run` — trigger execution
- `GET /api/ops/jobs/{job}` — get job details
- `DELETE /api/ops/jobs/{job}` — delete job
- `POST /api/ops/runs/{runId}/approve` — approve a waiting run
- `POST /api/ops/runs/{runId}/reject` — reject a waiting run
- `GET /api/ops/schedules` — list all schedules
- `POST /api/ops/schedules` — create a schedule
- `PUT /api/ops/schedules/{schedule}` — update a schedule
- `DELETE /api/ops/schedules/{schedule}` — delete a schedule
- `POST /api/ops/schedules/{schedule}/trigger` — trigger a scheduled run immediately
