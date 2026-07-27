# Runbooks

<img src="assets/images/desktop-runbooks-receipt.png" alt="Runbooks showing the immutable receipt for a successful fictitious telemetry recovery, beside the accepted procedure, approval boundary, execution result, and current target recheck" />

The Orbital Station procedure is fictitious showcase data captured from the
real Runbooks UI; Sentinel has no satellite-specific behavior.

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

## Explicit Procedures

Sentinel does not install default runbooks. Commands, service managers, PATH,
permissions, and recovery policy are host-specific, so every procedure must be
created explicitly for the environment where it will run.

## Parameters

Runbooks can define a `parameters` array. Each parameter has:

- `name` — identifier used in `{{NAME}}` placeholders
- `label` — human-readable label
- `type` — `string`, `number`, `boolean`, or `select`
- `default` — default value (used when the caller omits the parameter)
- `required` — when `true`, the run fails validation if the value is empty
- `options` — list of allowed values (for `select` type only)

When a run is triggered, supplied parameter values are merged with defaults. `{{PARAM}}` placeholders in step commands and scripts are replaced with shell-escaped values before execution. The resolved parameter map is persisted in the `parametersUsed` field of the run record and remains visible in the API, UI, and MCP output. Parameters are operational data, not secrets; do not put credentials or tokens in them.

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
  "targetService": "myapp",
  "parameters": [
    { "name": "SERVICE", "label": "Service name", "type": "string", "required": true }
  ],
  "steps": [
    { "type": "run", "title": "Check service", "command": "systemctl status {{SERVICE}}" },
    {
      "type": "script",
      "title": "Gather logs",
      "script": "#!/usr/bin/env bash\njournalctl -u {{SERVICE}} --no-pager -n 50"
    },
    {
      "type": "approval",
      "title": "Confirm restart",
      "description": "Review output before restarting."
    },
    {
      "type": "run",
      "title": "Restart",
      "command": "systemctl restart {{SERVICE}}",
      "continueOnError": true,
      "timeout": 60,
      "retries": 2,
      "retryDelay": 5
    }
  ]
}
```

Returns `201` with `{ runbook }`. The response may also include a `shellWarnings` array if any `run` or `script` steps contain shell syntax issues (validated via `mvdan.cc/sh`). Warnings are non-blocking — the runbook is still saved.

`targetService` is optional and must match a service currently tracked by
Sentinel. One service can belong to only one Runbook. A duplicate association
returns `409 OPS_RUNBOOK_TARGET_CONFLICT`; removing an associated custom service
returns `409 OPS_SERVICE_IN_USE`.

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

Every manual run opens a confirmation that shows the description, service
target, ordered steps, approval boundaries, and parameter persistence before
the job is created. A successful start immediately changes the route to the
returned `/runbooks?job=<id>` receipt.

Every new job persists its origin as `source=runbooks` for Runbooks/API/MCP,
`source=scheduler` for periodic and manually triggered schedules, or
`source=now` for procedures started from Now. When the definition has
`targetService`, the job also records `targetKind=service` and a copy of the
service name in `targetName`. Historical jobs created before this contract keep
these fields empty; Sentinel does not infer values retroactively.

Each new job also contains `definition`, an immutable, versioned receipt of the
name, description, steps, parameter definitions, webhook, and target used for
that execution. Running and approval resume use only this receipt, so later
edits or deletion of the runbook cannot change an in-flight execution.

The receipt remains addressable at `/runbooks?job=<id>`. The execution ID is
the canonical navigation key: if the current definition still exists, the UI
opens its history with that job expanded; if the definition was deleted, the
same URL renders the receipt standalone. The receipt distinguishes the stored
execution result from the current state of its service target. Current target
state is fetched only while the receipt is open and carries its own
`observedAt`; it never rewrites historical evidence.

Runbooks owns action and proof in the operational loop. Now and Services can
hand off a service target, but every manual execution still uses the same
effect-review dialog. The resulting receipt freezes the accepted definition,
source, parameters, and target; its live target recheck then closes the loop
without turning current state into historical output.

Only one queued, running, or waiting-for-approval job may own a service target
at a time. A competing start returns `409 RUNBOOK_TARGET_BUSY`; targetless jobs
continue to use only the global concurrency limit.

Job status lifecycle:

```text
queued -> running -> succeeded
                  -> failed
                  -> waiting_approval -> running -> ...
```

Only `succeeded` and `failed` are terminal. `waiting_approval` is a persisted
pause that can return to `running` after approval; rejection transitions it to
`failed`.

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

Runs paused at `waiting_approval` are persisted decision points. They remain pending across Sentinel restarts until an operator approves or rejects them, while continuing to reserve their service target.
Before the decision controls, the frontend shows the recorded target and the
remaining steps from `definition`, not from the current editable runbook.

During startup, jobs left in `queued` or `running` by an interrupted process are
reconciled to `failed`. Jobs in `waiting_approval` are deliberately preserved
instead of being treated as orphaned execution.

At each step completion, the job is updated in the store and an `ops.job.updated` event is emitted with the full job object including accumulated step results.

## Shell Validation

On create and update, Sentinel validates shell syntax for all `run` and `script` steps using `mvdan.cc/sh`. Warnings are returned in the response as a `shellWarnings` array:

```json
{
  "runbook": { "...": "..." },
  "shellWarnings": [{ "step": 0, "line": 1, "column": 12, "message": "unexpected token" }]
}
```

Shell warnings are advisory — they do not block saving the runbook.

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
    "targetKind": "service",
    "targetName": "myapp",
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
    "source": "runbooks",
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

Returns the persisted execution receipt independently from the recent-job
window and independently from whether its runbook definition still exists.

Delete a job:

```
DELETE /api/ops/jobs/{job}
```

## Scheduling

Runbooks can be executed on a schedule. Two schedule types are supported:

- **Cron** — recurring execution using standard cron expressions (e.g. `0 */6 * * *`). Supports optional timezone via IANA identifiers (e.g. `America/New_York`); defaults to the host's local timezone.
- **One-shot** — single future execution at a specific time, automatically removed after firing.

Schedules are managed via the API and the frontend editor. A background
scheduler evaluates pending schedules every five seconds and triggers runs as
they come due. Scheduled runs use `"source": "scheduler"` in job objects and
webhook payloads.

When a schedule is created, updated, or deleted, an `ops.schedule.updated` event is emitted over the `/ws/events` WebSocket.

## Realtime Events

- `ops.job.updated` — emitted on each state change (queued, running, per-step progress, waiting_approval, completion)
- Each event payload includes `{ globalRev, job }` with the full job object and accumulated `stepResults`
- `ops.schedule.updated` — emitted when a schedule is created, modified, or removed

WebSocket events are the primary update path. After starting or approving a
job, the frontend also runs a bounded settlement sequence at 250 ms, 750 ms,
1.5 s, 3 s, 6 s, and 10 s. It stops when the job is no longer active, when the
sequence is exhausted, or when a terminal WebSocket update supersedes it. This
is short-lived reconciliation, not continuous background polling.

## Frontend

The dedicated `/runbooks` route provides a standalone page for runbook execution and job history:

- Sidebar listing all runbooks with run counts
- Detail view showing step overview, typed service-target link, and a run button
- Job history cards expandable into immutable execution receipts
- Frozen steps correlated with stored output, errors, and duration
- Separate **Execution result** and on-demand **Current target state**
- Canonical `/runbooks?job=<id>` links that survive definition deletion
- Job deletion with inline confirmation
- Editor for creating and editing custom runbooks with drag-to-reorder steps
- Optional tracked-service selector with unique association enforcement
- Source and service target shown in history only when the run contains them
- Schedule management: create, edit, and delete cron or one-shot schedules per runbook

A typed `409 RUNBOOK_TARGET_BUSY` keeps the current procedure context open and
asks the operator to review the active execution before retrying.

## API Endpoints

- `GET /api/ops/runbooks` — list runbooks and recent jobs
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
