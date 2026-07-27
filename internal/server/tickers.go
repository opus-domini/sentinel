package server

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/services"
)

type metricsProvider interface {
	MetricsSnapshot(context.Context) services.MetricsSnapshot
}

type postureEventState struct {
	initialized bool
	signature   string
}

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

func startMetricsTicker(ctx context.Context, mgr metricsProvider, hub *events.Hub) <-chan struct{} {
	state := &postureEventState{}
	return loopTicker(ctx, 2*time.Second, func() {
		publishMetrics(ctx, mgr, hub, state)
	})
}

// publishMetrics samples host metrics and broadcasts them on the event hub.
func publishMetrics(
	ctx context.Context,
	mgr metricsProvider,
	hub *events.Hub,
	state *postureEventState,
) {
	collectCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	snapshot := mgr.MetricsSnapshot(collectCtx)
	cancel()
	hub.Publish(events.NewEvent(events.TypeOpsMetrics, map[string]any{
		"metrics": snapshot.Metrics,
		"posture": snapshot.Posture,
	}))

	signature := metricPostureSignature(snapshot.Posture)
	if state != nil && state.initialized && state.signature == signature {
		return
	}
	if state != nil {
		state.initialized = true
		state.signature = signature
	}
	hub.Publish(events.NewEvent(events.TypeOpsPosture, map[string]any{
		"posture": snapshot.Posture,
	}))
}

func metricPostureSignature(posture services.MetricPosture) string {
	signals := make([]string, 0, len(posture.Signals))
	for _, signal := range posture.Signals {
		signals = append(signals, signal.Name+":"+signal.Severity)
	}
	sort.Strings(signals)
	return strings.Join(
		[]string{posture.State, posture.Severity, strings.Join(signals, ",")},
		"|",
	)
}
