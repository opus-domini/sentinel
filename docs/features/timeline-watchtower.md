# Timeline and Watchtower

Watchtower is the background projection engine that converts tmux runtime output into structured activity state.

## Responsibilities

- Periodically inspect active tmux sessions/windows/panes.
- Capture pane tail content.
- Compute unread revisions.
- Project session/window/pane activity state into SQLite.
- Publish compact event patches to `/ws/events`.
- Write timeline events for operations search.

## Default Runtime Settings

- Tick interval: `1s`
- Pane capture lines: `80`
- Pane capture timeout: `150ms`
- Activity journal retention: `5000` rows
- Timeline retention: `20000` rows

These values are configurable (see configuration reference).

## Event Model

Watchtower emits:

- `tmux.sessions.updated`
- `tmux.activity.updated`
- `tmux.timeline.updated`

Payloads include compact `sessionPatches` and `inspectorPatches` to avoid expensive refetch loops.

## Delta Sync

HTTP fallback endpoint:

- `GET /api/tmux/activity/delta?since=<globalRev>&limit=<n>`

Used to reconcile missed events or WS reconnect gaps.

## Timeline Search

Endpoint:

- `GET /api/tmux/timeline`

Filters:

- `session`, `windowIndex`, `paneId`
- `q` (search text)
- `severity`, `eventType`
- `since`, `until` (RFC3339)
- `limit` (up to 500)

## Performance Notes

- Event-first architecture drastically reduces REST polling pressure.
- Delta sync is throttled and only used for reconciliation.
- Watchtower runtime stats are exposed via `GET /api/tmux/activity/stats`.
