package ops

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

const (
	defaultHealthInterval = 30 * time.Second
	cpuAlertThreshold     = 90.0
	memAlertThreshold     = 90.0
	diskAlertThreshold    = 95.0
)

// HealthPublisher emits events for real-time updates.
type HealthPublisher func(eventType string, payload map[string]any)

// HealthChecker periodically polls service states and host metrics,
// generating alerts on failures and auto-resolving on recovery.
type HealthChecker struct {
	manager  *Manager
	store    *store.Store
	publish  HealthPublisher
	interval time.Duration

	stopOnce sync.Once
	stopFn   context.CancelFunc
	doneCh   chan struct{}
}

// NewHealthChecker creates a health checker.
func NewHealthChecker(mgr *Manager, st *store.Store, publish HealthPublisher, interval time.Duration) *HealthChecker {
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	return &HealthChecker{
		manager:  mgr,
		store:    st,
		publish:  publish,
		interval: interval,
		doneCh:   make(chan struct{}),
	}
}

// Start begins the periodic health check loop.
func (hc *HealthChecker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	hc.stopFn = cancel
	go hc.loop(ctx)
}

// Stop gracefully stops the health checker.
func (hc *HealthChecker) Stop() {
	hc.stopOnce.Do(func() {
		if hc.stopFn != nil {
			hc.stopFn()
		}
		<-hc.doneCh
	})
}

func (hc *HealthChecker) loop(ctx context.Context) {
	defer close(hc.doneCh)

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.check(ctx)
		}
	}
}

func (hc *HealthChecker) check(ctx context.Context) {
	hc.checkServices(ctx)
	hc.checkMetrics(ctx)
}

func (hc *HealthChecker) checkServices(ctx context.Context) {
	if hc.manager == nil {
		return
	}
	services, err := hc.manager.ListServices(ctx)
	if err != nil {
		slog.Warn("health check: list services failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, svc := range services {
		state := strings.ToLower(strings.TrimSpace(svc.ActiveState))
		dedupeKey := fmt.Sprintf("health:service:%s:failed", svc.Name)

		switch state {
		case "failed":
			hc.raiseAlert(ctx, store.OpsAlertWrite{
				DedupeKey: dedupeKey,
				Source:    "health",
				Resource:  svc.Name,
				Title:     fmt.Sprintf("Service %s failed", svc.DisplayName),
				Message:   fmt.Sprintf("Service %s is in failed state (unit=%s)", svc.DisplayName, svc.Unit),
				Severity:  "error",
				Metadata:  fmt.Sprintf(`{"service":"%s","unit":"%s","state":"%s"}`, svc.Name, svc.Unit, state),
				CreatedAt: now,
			})
		case "active", "running":
			hc.resolveAlert(ctx, dedupeKey, now)
		}
	}
}

func (hc *HealthChecker) checkMetrics(ctx context.Context) {
	if hc.manager == nil {
		return
	}
	metrics := hc.manager.Metrics(ctx)
	now := time.Now().UTC()

	if metrics.CPUPercent > cpuAlertThreshold && metrics.CPUPercent >= 0 {
		hc.raiseAlert(ctx, store.OpsAlertWrite{
			DedupeKey: "health:host:cpu:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High CPU usage",
			Message:   fmt.Sprintf("CPU usage is %.1f%% (threshold: %.0f%%)", metrics.CPUPercent, cpuAlertThreshold),
			Severity:  "warn",
			Metadata:  fmt.Sprintf(`{"cpuPercent":%.1f}`, metrics.CPUPercent),
			CreatedAt: now,
		})
	} else if metrics.CPUPercent >= 0 {
		hc.resolveAlert(ctx, "health:host:cpu:high", now)
	}

	if metrics.MemPercent > memAlertThreshold {
		hc.raiseAlert(ctx, store.OpsAlertWrite{
			DedupeKey: "health:host:memory:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High memory usage",
			Message:   fmt.Sprintf("Memory usage is %.1f%% (threshold: %.0f%%)", metrics.MemPercent, memAlertThreshold),
			Severity:  "warn",
			Metadata:  fmt.Sprintf(`{"memPercent":%.1f}`, metrics.MemPercent),
			CreatedAt: now,
		})
	} else {
		hc.resolveAlert(ctx, "health:host:memory:high", now)
	}

	if metrics.DiskPercent > diskAlertThreshold {
		hc.raiseAlert(ctx, store.OpsAlertWrite{
			DedupeKey: "health:host:disk:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High disk usage",
			Message:   fmt.Sprintf("Disk usage is %.1f%% (threshold: %.0f%%)", metrics.DiskPercent, diskAlertThreshold),
			Severity:  "error",
			Metadata:  fmt.Sprintf(`{"diskPercent":%.1f}`, metrics.DiskPercent),
			CreatedAt: now,
		})
	} else {
		hc.resolveAlert(ctx, "health:host:disk:high", now)
	}
}

func (hc *HealthChecker) raiseAlert(ctx context.Context, write store.OpsAlertWrite) {
	if hc.store == nil {
		return
	}
	alert, err := hc.store.UpsertOpsAlert(ctx, write)
	if err != nil {
		slog.Warn("health check: upsert alert failed", "error", err)
		return
	}
	if hc.publish != nil {
		hc.publish("ops.alerts.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"alert":     alert,
		})
	}
}

func (hc *HealthChecker) resolveAlert(ctx context.Context, dedupeKey string, at time.Time) {
	if hc.store == nil {
		return
	}
	alert, err := hc.store.ResolveOpsAlert(ctx, dedupeKey, at)
	if err != nil {
		return // Not found is expected when no prior alert exists.
	}
	if hc.publish != nil {
		hc.publish("ops.alerts.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"alert":     alert,
		})
	}
}
