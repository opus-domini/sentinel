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

func startMetricsTicker(ctx context.Context, mgr *services.Manager, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
				m := mgr.Metrics(collectCtx)
				cancel()
				hub.Publish(events.NewEvent(events.TypeOpsMetrics, map[string]any{
					"metrics": m,
				}))
			}
		}
	}()
	return done
}

func startAlertsTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastRev int64
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				rev, err := st.GetOpsAlertRevision(collectCtx)
				if err != nil {
					cancel()
					slog.Warn("alerts tick: revision check failed", "err", err)
					continue
				}
				if rev == lastRev {
					cancel()
					continue
				}
				alerts, err := st.ListAlerts(collectCtx, 100, "")
				cancel()
				if err != nil {
					slog.Warn("alerts tick failed", "err", err)
					continue
				}
				lastRev = rev
				hub.Publish(events.NewEvent(events.TypeOpsAlerts, map[string]any{
					"alerts": alerts,
				}))
			}
		}
	}()
	return done
}

func startActivityTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastRev int64
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				rev, err := st.GetOpsActivityRevision(collectCtx)
				if err != nil {
					cancel()
					slog.Warn("activity tick: revision check failed", "err", err)
					continue
				}
				if rev == lastRev {
					cancel()
					continue
				}
				result, err := st.SearchActivityEvents(collectCtx, activity.Query{
					Limit: 200,
				})
				cancel()
				if err != nil {
					slog.Warn("activity tick failed", "err", err)
					continue
				}
				lastRev = rev
				hub.Publish(events.NewEvent(events.TypeOpsActivity, map[string]any{
					"events": result.Events,
				}))
			}
		}
	}()
	return done
}

func startOpsPruneTicker(ctx context.Context, st *store.Store) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if n, err := st.PruneOpsActivityRows(pruneCtx, 10000); err != nil {
					slog.Warn("ops activity prune failed", "err", err)
				} else if n > 0 {
					slog.Info("ops activity pruned", "removed", n)
				}
				cancel()
			}
		}
	}()
	return done
}
