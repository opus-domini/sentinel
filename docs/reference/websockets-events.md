# WebSocket and Events Reference

Sentinel exposes three WS endpoints.

## Endpoints

| Endpoint                  | Purpose                      |
| ------------------------- | ---------------------------- |
| `/ws/tmux?session=<name>` | Attach to tmux session PTY   |
| `/ws/events`              | Realtime state/event channel |
| `/ws/logs?service=<name>` | Service log streaming        |

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
- `ops.overview.updated`
- `ops.services.updated`
- `ops.metrics.updated`
- `ops.posture.updated`
- `ops.schedule.updated`
- `ops.job.updated`

`ops.metrics.updated` carries the same cacheable shape as
`GET /api/ops/metrics`, evaluated from one sample:

```json
{
  "type": "ops.metrics.updated",
  "payload": {
    "metrics": {
      "cpuPercent": 85,
      "collectedAt": "2026-07-27T12:00:00Z"
    },
    "posture": {
      "state": "pressure",
      "severity": "warning",
      "warningCount": 1,
      "criticalCount": 0,
      "signals": [
        {
          "name": "cpu",
          "severity": "warning",
          "value": 85,
          "since": "2026-07-27T11:59:50Z"
        }
      ],
      "observedAt": "2026-07-27T12:00:00Z"
    }
  }
}
```

`ops.metrics.updated` is emitted for every sample even when only numeric values
change. `ops.posture.updated` carries `{ "posture": ... }` only when `state`,
`severity`, or the active `name+severity` signal set changes. `since` and
`observedAt` are evidence timestamps; numeric value changes alone do not create
a semantic posture event.

### Now invalidation

Now does not publish or subscribe to a dedicated `now.updated` event. The
frontend invalidates `GET /api/now` when it receives
`tmux.sessions.updated`, `ops.services.updated`, `ops.overview.updated`,
`ops.posture.updated`, or `ops.job.updated`. No event means no periodic Now
request; reconnect and explicit resync are the fallback paths.

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
