# WebSocket and Events Reference

Sentinel exposes three WS endpoints.

## Endpoints

| Endpoint | Purpose |
| --- | --- |
| `/ws/tmux?session=<name>` | Attach to tmux session PTY |
| `/ws/events` | Realtime state/event channel |
| `/ws/logs?service=<name>` | Service log streaming |

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

- `events.ready`
- `tmux.sessions.updated`
- `tmux.inspector.updated`
- `tmux.activity.updated`
- `tmux.timeline.updated`
- `tmux.guardrail.blocked`
- `ops.overview.updated`
- `ops.services.updated`
- `ops.alerts.updated`
- `ops.activity.updated`
- `ops.metrics.updated`
- `ops.schedule.updated`
- `ops.job.updated`
- `recovery.overview.updated`
- `recovery.job.updated`

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
- Gap/reconnect fallback: `GET /api/tmux/activity/delta`.
- Full fallback polling is used only when events WS is disconnected.
