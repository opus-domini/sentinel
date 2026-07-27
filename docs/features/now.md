# Now

Now is Sentinel's operational home at `/`. It is the entrance and return point
for the daily loop, not another owner module.

## What Now Answers

One current snapshot answers:

1. Is current host evidence healthy, at risk, or unable to confirm posture?
2. Which decisions need a trusted operator now?
3. Which procedures or terminal contexts are already in progress?
4. Which owner can answer the next question precisely?

Now composes Services, Metrics, Runbooks, and Tmux concurrently. Partial failure
preserves healthy owner evidence and identifies the source whose freshness
could not be confirmed.

## Posture and Confidence

Posture derives from current Services and Metrics evidence:

- **Healthy** — no current failed tracked service or pressure signal.
- **At risk** — current service or pressure evidence needs attention.
- **Unknown** — Services or Metrics cannot confirm current posture.

Confidence describes all four sources. It is **Current** only when every owner
is current and **Degraded** when any source is stale, unavailable, or not
configured. A Tmux or Runbooks failure can degrade confidence without turning
otherwise healthy service/metric evidence into risk.

## Needs Attention

The decision queue admits only current evidence with a defined handoff:

- a failed tracked service opens its structured status in Services;
- host pressure opens the highest-severity signal in Metrics;
- a waiting approval opens the exact execution in Runbooks.

The queue is bounded and reports hidden counts by owner. Historical terminal
job failures do not become permanent urgency. Now does not infer which process
caused pressure, restart a service, acknowledge an alert, or create an
incident.

When one enabled Runbook is associated with a failed service, Now may open the
normal Runbook confirmation flow. Execution, target admission, approval,
persistence, and receipt remain owned by Runbooks.

## In Progress

Live context includes bounded sets of:

- queued or running Runbook executions;
- existing Tmux sessions with unread windows or panes.

Each item opens the exact owner resource. An approval remains a decision, not
autonomous progress. Pinned metadata alone does not make a quiet session active
work.

## Handoffs

Owner links are reload-safe and validated against current data. They can carry
the service and panel, metric signal and observation time, Runbook or job ID,
or existing Tmux session. Invalid or disappeared targets are handled by the
owner route rather than creating synthetic state.

See [Operational Loop](/features/operational-loop.md) for the cross-module
journey and [HTTP API](/reference/http-api.md) for exact endpoint and search
contracts.

## Refresh Model

HTTP provides the initial composition. Existing owner events invalidate Now;
there is no independent `now.updated` event or periodic Now collector. If the
shared event channel disconnects while a snapshot remains visible, current
source labels are presented as stale until a successful refresh.

## Boundary

Now owns composition, prioritization, confidence, and handoff. It has no
database table, durable workflow, background collector, alert engine, incident
engine, recovery timeline, or duplicate owner controls.
