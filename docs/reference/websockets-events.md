# WebSocket and Events Reference

Sentinel exposes three WS endpoints.

## Endpoints

| Endpoint | Purpose |
| --- | --- |
| `/ws/tmux?session=<name>` | Attach to a Tmux session PTY |
| `/ws/events` | Realtime state/event channel |
| `/ws/logs?service=<name>` | Stream logs for a tracked service |
| `/ws/logs?unit=<unit>&scope=<scope>&manager=<manager>` | Stream logs for a direct unit |

## Authentication

WS auth uses the same HttpOnly cookie (`sentinel_auth`) as HTTP requests. No token in URL query params.

## PTY Streams (`/ws/tmux`)

Server -> client:

- Initial JSON status message (`type: "status"`, `state: "attached"`, ids)
- Binary frames with terminal output

Client -> server:

- Binary frames with terminal input bytes
- Optional text control frame for resize:

```json
{ "type": "resize", "cols": 160, "rows": 42 }
```

## Events Channel (`/ws/events`)

### Initial message

Server sends:

```json
{ "type": "events.ready", "payload": { "message": "subscribed" } }
```

### Event envelope

```json
{
  "eventId": 123,
  "type": "tmux.sessions.updated",
  "timestamp": "2026-02-15T12:00:00Z",
  "payload": {}
}
```

`eventId` is monotonic and used by frontend to detect gaps.

### Published event types

| Type | Payload responsibility |
| --- | --- |
| `events.ready` | Subscription acknowledgement |
| `tmux.sessions.updated` | Session projection mutation or replacement |
| `tmux.inspector.updated` | Window/pane projection mutation |
| `tmux.activity.updated` | Tmux activity runtime statistics |
| `ops.overview.updated` | Current overview resource |
| `ops.services.updated` | Current tracked Services resource or invalidation |
| `ops.metrics.updated` | One current `{ metrics, posture }` sample |
| `ops.posture.updated` | Semantic posture transition |
| `ops.runbooks.updated` | Runbook definition collection changed |
| `ops.schedule.updated` | Schedule collection changed |
| `ops.job.updated` | Full current job after a state/step transition |

The event envelope and this type registry are canonical here. Owner semantics
belong to [Tmux](/features/tmux-workspace.md),
[Services](/features/services.md), [Metrics](/features/metrics.md), and
[Runbooks](/features/runbooks.md). Exact HTTP resource shapes remain in the
[HTTP API Reference](/reference/http-api.md).

### Now invalidation

Now has no `now.updated` event. It invalidates its composed read only from
semantic owner events; raw metric samples and overview refreshes do not create a
second composition stream. See [Now](/features/now.md) and the
[Operational Loop](/features/operational-loop.md) for the owner-level behavior.

### Client messages to `/ws/events`

Presence update:

```json
{
  "type": "presence",
  "terminalId": "...",
  "session": "dev",
  "windowIndex": 1,
  "paneId": "%4",
  "visible": true,
  "focused": true
}
```

Seen acknowledgement request:

```json
{
  "type": "seen",
  "requestId": "seen-...",
  "session": "dev",
  "scope": "pane",
  "windowIndex": 1,
  "paneId": "%4"
}
```

Seen ack response (`type: tmux.seen.ack`) includes `acked`, `globalRev`, and optional projection patches.

## Reconciliation Strategy

- Primary sync: WS events.
- Gap/reconnect fallback: reload the relevant HTTP resource.
- Full fallback polling is used only when events WS is disconnected.
