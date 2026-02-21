# Timeline and Watchtower

![Desktop timeline](assets/images/desktop-timeline.png)

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
- Timeline retention: `10000` rows

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

- `q` (search text)
- `severity`
- `source`
- `limit` (up to 500)

## Timeline Page

The dedicated `/activities` route provides a full-page view of the operational audit log.

- The sidebar displays overview stats (host, uptime, events count, health), a search query input, and a severity filter dropdown (all/info/warn/error). A `?` help dialog button and token/lock controls are available at the top of the sidebar.
- The main panel shows a scrollable list of timeline events. Each event displays: message, severity badge, source/resource, timestamp, and optional details.
- New events prepend in real-time via `ops.activity.updated` WebSocket events.
- Filters support text search query, severity level, and source.

Endpoint:

- `GET /api/ops/activity` -- search and filter operational activity events

## Performance Notes

- Event-first architecture drastically reduces REST polling pressure.
- Delta sync is throttled and only used for reconciliation.
- Watchtower runtime stats are exposed via `GET /api/tmux/activity/stats`.
