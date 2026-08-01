# MCP Control

Sentinel can expose its tmux and runbook control planes as a Streamable HTTP MCP
server at `/mcp`. This is intended for agents that need to work inside a remote
machine's existing tmux sessions or execute its operational runbooks without SSH
access.

The server uses the official
[Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk)
and is disabled by default.

## Enable

Configure the existing Sentinel token and enable MCP:

```toml
[server]
token = "strong-secret"

[mcp]
enabled = true
```

The MCP enabled state is available in **Settings > Integrations > MCP** and
through `SENTINEL_MCP_ENABLED=true`. Integrations shows the endpoint, runtime
availability, shared-token readiness, and ready-to-copy client configurations.
The shared token itself is configured only in **Settings > Access** so every
rotation uses reconnect validation and the anti-lockout review.

The shared token is write-only in Settings. Existing values are never
displayed. Choose Keep, Replace, or Clear; a replacement is removed from the
browser form immediately after submit and becomes active only after restart.
If the process started without a token, replace it in Access, restart Sentinel,
and then enable MCP from Integrations. MCP disable remains live.

Every MCP request must send:

```http
Authorization: Bearer strong-secret
```

The Sentinel login cookie is not an MCP credential. There is no separate MCP
token.

## Trust Boundary

The Bearer value is the same shared `server.token` used by trusted browser
operators. Sentinel does not derive an agent identity from it and has no
per-agent users, roles, scopes, or resource grants. Every MCP client holding the
secret enters the same trusted boundary.

An ephemeral Tmux session has an opaque lifecycle lease. That lease is a handle
for one exact Tmux runtime ID; it is not an agent identity, credential, role, or
ownership boundary. Any trusted MCP client with the shared token and the exact
lease confirmation can keep or close that resource.

The optional `user` accepted by Tmux tools is an
[OS account target](/features/os-account-targeting.md), not the identity of the
calling agent. Target validation still applies, but it does not create an
attributable Sentinel principal.

Runbook approval and rejection are intentionally unavailable to MCP tools.
`runbook_wait` can observe `waiting_approval`; only a human using the
authenticated Sentinel UI can resolve that decision.

## Connect

For a host named `azdrix` serving Sentinel at `https://azdrix.example.ts.net`:

### Codex

```bash
export SENTINEL_TOKEN='<same value as server.token>'
codex mcp add sentinel-azdrix \
  --url https://azdrix.example.ts.net/mcp \
  --bearer-token-env-var SENTINEL_TOKEN
```

### Claude Code

Claude Code expands environment variables in HTTP headers stored in its MCP
configuration:

```bash
export SENTINEL_TOKEN='<same value as server.token>'
claude mcp add-json --scope user sentinel-azdrix \
  '{"type":"http","url":"https://azdrix.example.ts.net/mcp","headers":{"Authorization":"Bearer ${SENTINEL_TOKEN}"}}'
```

### `mcpServers` JSON

```json
{
  "mcpServers": {
    "sentinel-azdrix": {
      "type": "http",
      "url": "https://azdrix.example.ts.net/mcp",
      "headers": {
        "Authorization": "Bearer ${SENTINEL_TOKEN}"
      }
    }
  }
}
```

Environment-variable expansion in a generic `mcpServers` file depends on the
client. Do not commit the literal token.

The Settings page derives the MCP server name from the Sentinel host name and
uses the same `sentinel-<hostname>` identifier in every client format.

## Tools

| Tool | Purpose |
| --- | --- |
| `tmux_list_sessions` | List sessions visible to Sentinel |
| `tmux_create_session` | Create a detached session; ephemeral for 2 hours of idle time by default, or persistent by explicit override |
| `tmux_list_windows` | Inspect stable window IDs and metadata |
| `tmux_list_panes` | Inspect pane IDs, commands, paths, and geometry |
| `tmux_attach` | Open a native tmux control-mode attachment and capture the active pane |
| `tmux_interact` | Send ordered literal-text and named-key actions, then wait and capture the pane |
| `tmux_read` | Long-poll incremental control events after a cursor |
| `tmux_detach` | Release the MCP attachment without killing the tmux session |
| `tmux_keep_session` | Promote one exact ephemeral session to persistent |
| `tmux_close_session` | Close one exact ephemeral session by lease, confirmed name, and stable runtime ID |
| `runbook_list` | List runbooks, parameters, step counts, and approval requirements |
| `runbook_get` | Read a complete runbook definition |
| `runbook_create` | Validate and create a runbook without executing it |
| `runbook_delete` | Delete a confirmed inactive runbook and its schedules while preserving run history |
| `runbook_run` | Start a runbook with typed parameter values |
| `runbook_get_run` | Inspect one execution with bounded trailing step output |
| `runbook_wait` | Wait for progress, completion, or a human approval boundary |
| `runbook_list_runs` | List recent executions with bounded trailing step output |

There is deliberately no raw tmux-command tool.

## Tmux Session Lifecycle

`tmux_create_session` accepts an optional `lifetime` with only two values:

- omitted or `ephemeral`: create a persisted lifecycle lease with a 2-hour
  sliding idle deadline;
- `persistent`: create an ordinary durable Tmux session with no lifecycle
  lease.

There is no per-session TTL input. Sessions created by the SPA, HTTP API,
pinned presets, launchers, startup restore, or outside Sentinel remain
persistent and are never adopted by the MCP lifecycle controller.

An ephemeral create returns lifecycle data alongside the session:

```json
{
  "lifecycle": {
    "mode": "ephemeral",
    "source": "mcp",
    "leaseId": "lease_...",
    "cleanupState": "active",
    "idleTimeoutSeconds": 7200,
    "expiresAt": "2026-08-01T18:00:00Z"
  }
}
```

`tmux_list_sessions` reports lifecycle data only when the persisted lease still
matches the live session's stable Tmux ID. It does not renew any deadline.
Sessions without lifecycle data are persistent or unmanaged.

The deadline is renewed after these directed operations succeed:
`tmux_attach`, `tmux_interact`, `tmux_read`, `tmux_list_windows`, and
`tmux_list_panes`. A normal `tmux_read` timeout renews; a closed stream or a
failed operation does not. Global listing, detach, runbook tools, spontaneous
output, a live process, and human activity do not renew a session.

After 2 hours of inactivity, cleanup enters a 10-minute grace period. New
successful directed activity during grace reactivates the lease for another 2
hours. When grace ends, Sentinel drains its MCP attachments and checks the
exact stable runtime ID again. A remaining human or external Tmux client blocks
cleanup until that client leaves; a missing session or reused name removes only
the stale lease and never kills the replacement.

Use the exact `leaseId` and current session name for explicit transitions:

```json
{ "leaseId": "lease_...", "confirmName": "agent-work" }
```

- `tmux_keep_session` removes the lease without stopping the session. The
  session becomes persistent.
- `tmux_close_session` drains MCP attachments and kills only the stable Tmux ID
  recorded in the lease.

A persistent session has no lease, so neither tool can adopt or close it. For a
long-running job that may be silent for more than 2 hours, create it with
`"lifetime": "persistent"` or call `tmux_keep_session` before leaving it
unattended. Do not rely on process output or mere liveness as renewal.

There is also deliberately no MCP tool to update a runbook. Agents can create a
new explicit definition. As described by the trust boundary above,
`runbook_wait` returns immediately when a run reaches `waiting_approval` so the
agent can report that boundary instead of hanging.

## Runbook Model

A typical agent flow is:

1. Call `runbook_list`, then `runbook_get` when the full definition is needed.
2. Call `runbook_run` with values matching the runbook's typed parameters.
3. Call `runbook_wait`, passing the last `completedSteps` value as
   `afterCompletedSteps` when following a longer execution.
4. Continue waiting until the status is `succeeded`, `failed`, or
   `waiting_approval`.

`runbook_wait` is a bounded long poll capped at 20 seconds. `runbook_get_run`,
`runbook_wait`, and `runbook_list_runs` return only the trailing portion of each
step's output (4,000 characters by default, configurable up to 32,768) and mark
truncated output with `outputTruncated: true`. Their run objects also include
the immutable `definition` receipt used by the execution. Resolved parameter
values are persisted and visible; do not pass credentials or tokens as
runbook parameters.

`runbook_create` defaults `enabled` to `true`, performs the same definition
validation as the HTTP API, and returns non-blocking shell syntax warnings.
`runbook_delete` requires `confirmName` to exactly match the persisted name,
refuses deletion while an execution is queued, running, or waiting for approval,
and preserves historical executions.

Starting a run whose service target already has a queued, running, or
waiting-for-approval execution fails with `target service already has an active
execution`. Runs without a service target remain governed only by the global
concurrency limit.

MCP uses the same Runbook Manager as Now, the HTTP API, and the Scheduler.
Therefore target ownership, immutable receipts, persisted source metadata, and
the prohibition on agent approval are identical across entry points; MCP does
not create a parallel execution path.

## Interaction Model

`tmux_attach` returns an `attachmentId`, active `paneId`, event `cursor`, and
the current visible screen. Use the stable pane ID for subsequent calls.

`tmux_interact` accepts an ordered list so text and special keys are explicit:

```json
{
  "attachmentId": "att_...",
  "paneId": "%12",
  "input": [
    { "type": "text", "value": "npm test" },
    { "type": "key", "value": "Enter" }
  ],
  "wait": {
    "mode": "idle",
    "quietMs": 400,
    "timeoutMs": 5000
  }
}
```

Wait modes:

- `none`: return immediately after sending input;
- `idle`: return after no matching control events arrive for `quietMs`;
- `text`: return when the visible screen contains `pattern`, optionally as a
  regular expression.

Waits are capped at 20 seconds. `settled: true` only means the pane was quiet
for the requested interval; it does not claim that the process or command has
finished. Use `tmux_read` with the returned cursor to continue following output.

Attachments to the same OS user and tmux session share one native control-mode
client. Each caller gets an independent 30-minute attachment lease. This
in-memory client lease is distinct from the persisted 2-hour session lifecycle
lease: attachment expiry releases control-mode resources but does not by itself
kill the Tmux session. Output is kept in a bounded event buffer, and
`droppedEvents` reports when a cursor fell behind that buffer.

`tmux_detach` only closes the lease. It never kills the tmux session.
