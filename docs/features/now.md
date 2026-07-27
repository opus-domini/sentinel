# Now

![Desktop Now](assets/images/desktop-now.png)

Now is Sentinel's operational home at `/`. It answers three questions from one
current snapshot:

1. Is the host operational?
2. What needs an operator decision?
3. What work is already in progress?

It composes evidence from Services, Metrics, Runbooks, and Tmux without taking
ownership away from those modules.

## Posture and Confidence

The headline reports host posture separately from evidence confidence:

- **Healthy** — current Services and Metrics evidence contains no failed
  service or metric pressure.
- **At risk** — current Services or Metrics evidence needs operator attention.
- **Unknown** — Services or Metrics cannot confirm the current posture.

Confidence is **Current** only when all four owner sources are current. It is
**Degraded** when Tmux, Services, Metrics, or Runbooks is stale, unavailable,
or not configured. A Tmux or Runbooks problem degrades confidence without
changing an otherwise confirmed host posture; a Services or Metrics problem
makes posture unknown.

Each source shows its owner-provided `observedAt` and remains a link to its
owner page. A partial response preserves valid evidence and identifies the
source that could not confirm current state. When the events connection drops
after a snapshot was loaded, current source labels become stale, confidence
becomes degraded, and posture becomes unknown until a successful refresh.

## Needs Attention

Now admits only current failed services, pending Runbook approvals, and current
Metrics pressure. A terminal Runbook failure is execution history, not present
urgency, so it never enters this queue.

At most five items are visible. The first pass reserves representation for
each non-empty category in operational order: failed service, critical Metrics,
approval, then warning Metrics. Remaining capacity goes to failed services by
canonical name and then the oldest approvals. Metrics contributes at most one
item. Hidden counts are reported by owner module.

Now does not restart services. A failed service opens its status in Services,
where `View logs` continues to the live log panel. When exactly one enabled
Runbook is associated with the failed service, `Run procedure` opens the normal
parameter dialog and starts that procedure through the Runbook execution
engine. A successful start immediately opens the returned immutable execution
receipt in Runbooks. A target already owned by another active execution keeps
the dialog open and names that conflict instead of reporting a generic error.

## In Progress

The live-context panel shows:

- Up to three queued or running Runbook executions.
- Up to three existing Tmux sessions with unread windows or panes.

Each item opens the exact owner resource. Approvals stay in Needs attention
because they require a decision rather than representing autonomous progress.
Pinned remains session metadata; it does not make a quiet session current work.

## Deep Links

Owner links are reload-safe and validated against current data:

| Owner             | Search parameters                             |
| ----------------- | --------------------------------------------- |
| Services          | `/services?service=<name>&panel=status\|logs` |
| Runbook definition | `/runbooks?runbook=<id>`                     |
| Runbook execution | `/runbooks?job=<id>`                          |
| Tmux              | `/tmux?session=<name>`                        |

Invalid definitions and disappeared Services or Tmux targets are removed from
the URL with history replacement. A missing execution keeps its canonical URL
and shows an explicit unavailable state. Tmux never creates a tab from a URL
target; the session must already exist.

## Refresh Model

`GET /api/now` provides the initial snapshot. The shared `/ws/events`
connection invalidates it for:

- `tmux.sessions.updated`
- `ops.services.updated`
- `ops.posture.updated`
- `ops.job.updated`
- `ops.runbooks.updated`

There is no `now.updated` event and no periodic polling. Reconnect and the
header resync control request a fresh snapshot explicitly.

Services owns a five-second state watcher and emits only when its canonical
fingerprint changes. Metrics emits `ops.metrics.updated` for every sample, but
Now listens only to semantic `ops.posture.updated` changes. Runbook definition
create, update, and delete operations emit `ops.runbooks.updated` through the
shared manager used by HTTP and MCP.

## Boundaries

Now is a read and handoff layer. It has no table, durable state, background
collector, timeline, alert engine, recovery engine, or duplicate dashboard.
Services owns status, logs, and lifecycle actions; Runbooks owns execution and
approvals; Metrics owns canonical host posture; Tmux owns session interaction.
