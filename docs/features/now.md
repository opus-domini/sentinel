# Now

![Desktop Now](assets/images/desktop-now.png)

Now is Sentinel's operational home at `/`. It answers three questions from one
current snapshot:

1. Is the host operational?
2. What needs an operator decision?
3. What work is already in progress?

It composes evidence from Services, Metrics, Runbooks, and Tmux without taking
ownership away from those modules.

## Reliability

The top panel shows the overall state and the freshness of each source:

- **Operational** — all available evidence is current and no actionable signal exists.
- **Needs attention** — evidence is current, but at least one item needs action.
- **Degraded** — one or more sources are stale, unavailable, or not configured.

Each source remains a link to its owner page. A partial response keeps valid
data visible and identifies the source that could not confirm current state.
When the events connection drops after a snapshot was loaded, current source
labels become stale until an explicit resync or successful reconnect refreshes
the snapshot.

## Needs Attention

Now presents at most five items in this fixed order:

1. Runbook approvals.
2. Failed services.
3. Latest failed Runbook executions.
4. Metrics pressure.

Items outside the visible limit are counted by owner module. A failed Runbook
execution targeting the same failed service is folded into the service item
instead of appearing twice.

Now does not restart services. A failed service opens its status in Services,
where `View logs` continues to the live log panel. When exactly one enabled
Runbook is associated with the failed service, `Run procedure` opens the normal
parameter dialog and starts that procedure through the Runbook execution
engine.

## In Progress

The live-context panel shows:

- Up to three queued or running Runbook executions.
- Up to three existing Tmux sessions that are pinned or have unread activity.

Each item opens the exact owner resource. Approvals stay in Needs attention
because they require a decision rather than representing autonomous progress.

## Deep Links

Owner links are reload-safe and validated against current data:

| Owner    | Search parameters                                      |
| -------- | ------------------------------------------------------ |
| Services | `/services?service=<name>&panel=status\|logs`          |
| Runbooks | `/runbooks?runbook=<id>&job=<id>`                     |
| Tmux     | `/tmux?session=<name>`                                |

Invalid or disappeared targets are removed from the URL with history
replacement. Tmux never creates a tab from a URL target; the session must
already exist.

## Refresh Model

`GET /api/now` provides the initial snapshot. The shared `/ws/events`
connection invalidates it for:

- `tmux.sessions.updated`
- `ops.services.updated`
- `ops.overview.updated`
- `ops.metrics.updated`
- `ops.job.updated`

There is no `now.updated` event and no periodic polling. Reconnect and the
header resync control request a fresh snapshot explicitly.

## Boundaries

Now is a read and handoff layer. It has no table, durable state, background
collector, timeline, alert engine, recovery engine, or duplicate dashboard.
Services owns status, logs, and lifecycle actions; Runbooks owns execution and
approvals; Metrics owns canonical host posture; Tmux owns session interaction.
