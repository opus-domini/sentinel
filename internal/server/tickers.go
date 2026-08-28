package server

import (
	"context"
	"encoding/json"
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

type servicesProvider interface {
	ListServices(context.Context) ([]services.ServiceStatus, error)
}

type servicesEventState struct {
	initialized bool
	fingerprint string
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

func startServicesWatcher(
	ctx context.Context,
	mgr servicesProvider,
	hub *events.Hub,
) <-chan struct{} {
	state := &servicesEventState{}
	return loopTicker(ctx, 5*time.Second, func() {
		_ = publishServicesIfChanged(ctx, mgr, hub, state)
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
		signals = append(signals, signal.Name+":"+signal.Subject+":"+signal.Severity)
	}
	sort.Strings(signals)
	return strings.Join(
		[]string{posture.State, posture.Severity, strings.Join(signals, ",")},
		"|",
	)
}

func publishServicesIfChanged(
	ctx context.Context,
	mgr servicesProvider,
	hub *events.Hub,
	state *servicesEventState,
) error {
	collectCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	current, err := mgr.ListServices(collectCtx)
	cancel()
	if err != nil {
		return err
	}
	if current == nil {
		current = []services.ServiceStatus{}
	}

	fingerprint := servicesFingerprint(current)
	if state != nil && !state.initialized {
		state.initialized = true
		state.fingerprint = fingerprint
		return nil
	}
	if state != nil && state.fingerprint == fingerprint {
		return nil
	}
	if state != nil {
		state.fingerprint = fingerprint
	}
	hub.Publish(events.NewEvent(events.TypeOpsServices, map[string]any{
		"globalRev": time.Now().UTC().UnixMilli(),
		"services":  current,
	}))
	return nil
}

func servicesFingerprint(current []services.ServiceStatus) string {
	type fingerprintService struct {
		Name         string `json:"name"`
		Manager      string `json:"manager"`
		Scope        string `json:"scope"`
		Unit         string `json:"unit"`
		TrackingMode string `json:"trackingMode"`
		Exists       bool   `json:"exists"`
		EnabledState string `json:"enabledState"`
		ActiveState  string `json:"activeState"`
	}

	normalized := make([]fingerprintService, 0, len(current))
	for _, service := range current {
		normalized = append(normalized, fingerprintService{
			Name:         service.Name,
			Manager:      service.Manager,
			Scope:        service.Scope,
			Unit:         service.Unit,
			TrackingMode: service.TrackingMode,
			Exists:       service.Exists,
			EnabledState: service.EnabledState,
			ActiveState:  service.ActiveState,
		})
	}
	sort.Slice(normalized, func(i, j int) bool {
		left, _ := json.Marshal(normalized[i])
		right, _ := json.Marshal(normalized[j])
		return string(left) < string(right)
	})
	encoded, _ := json.Marshal(normalized)
	return string(encoded)
}
