package server

import (
	"context"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/services"
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
