package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

// loopTicker runs tick every interval until ctx is cancelled. The returned
// channel closes once the loop has stopped, so shutdown can wait on it.
func loopTicker(ctx context.Context, interval time.Duration, tick func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tick()
			}
		}
	}()
	return done
}

func startMetricsTicker(ctx context.Context, mgr *services.Manager, hub *events.Hub) <-chan struct{} {
	return loopTicker(ctx, 2*time.Second, func() {
		publishMetrics(ctx, mgr, hub)
	})
}

// publishMetrics samples host metrics and broadcasts them on the event hub.
func publishMetrics(ctx context.Context, mgr *services.Manager, hub *events.Hub) {
	collectCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	m := mgr.Metrics(collectCtx)
	cancel()
	hub.Publish(events.NewEvent(events.TypeOpsMetrics, map[string]any{
		"metrics": m,
	}))
}

func startAlertsTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	var lastRev int64
	return loopTicker(ctx, 5*time.Second, func() {
		lastRev = publishAlertsIfChanged(ctx, st, hub, lastRev)
	})
}

// publishAlertsIfChanged broadcasts the alert list when the alert revision has
// advanced past lastRev. It returns the revision to remember for the next tick.
func publishAlertsIfChanged(ctx context.Context, st *store.Store, hub *events.Hub, lastRev int64) int64 {
	collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rev, err := st.GetOpsAlertRevision(collectCtx)
	if err != nil {
		slog.Warn("alerts tick: revision check failed", "err", err)
		return lastRev
	}
	if rev == lastRev {
		return lastRev
	}
	alerts, err := st.ListAlerts(collectCtx, 100, "")
	if err != nil {
		slog.Warn("alerts tick failed", "err", err)
		return lastRev
	}
	hub.Publish(events.NewEvent(events.TypeOpsAlerts, map[string]any{
		"alerts": alerts,
	}))
	return rev
}

func startActivityTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	var lastRev int64
	return loopTicker(ctx, 5*time.Second, func() {
		lastRev = publishActivityIfChanged(ctx, st, hub, lastRev)
	})
}

// publishActivityIfChanged broadcasts recent activity events when the activity
// revision has advanced past lastRev. It returns the revision to remember.
func publishActivityIfChanged(ctx context.Context, st *store.Store, hub *events.Hub, lastRev int64) int64 {
	collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rev, err := st.GetOpsActivityRevision(collectCtx)
	if err != nil {
		slog.Warn("activity tick: revision check failed", "err", err)
		return lastRev
	}
	if rev == lastRev {
		return lastRev
	}
	result, err := st.SearchActivityEvents(collectCtx, activity.Query{
		Limit: 200,
	})
	if err != nil {
		slog.Warn("activity tick failed", "err", err)
		return lastRev
	}
	hub.Publish(events.NewEvent(events.TypeOpsActivity, map[string]any{
		"events": result.Events,
	}))
	return rev
}

func startOpsPruneTicker(ctx context.Context, st *store.Store) <-chan struct{} {
	return loopTicker(ctx, 1*time.Hour, func() {
		pruneOpsActivity(ctx, st)
	})
}

// pruneOpsActivity trims the ops activity log to its retention cap.
func pruneOpsActivity(ctx context.Context, st *store.Store) {
	pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if n, err := st.PruneOpsActivityRows(pruneCtx, 10000); err != nil {
		slog.Warn("ops activity prune failed", "err", err)
	} else if n > 0 {
		slog.Info("ops activity pruned", "removed", n)
	}
}
