package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
)

const defaultHealthInterval = 30 * time.Second

// AlertThresholds configures the metric thresholds that trigger alerts.
type AlertThresholds struct {
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
}

// HealthPublisher emits events for real-time updates.
type HealthPublisher func(eventType string, payload map[string]any)

// healthAlertsRepo defines the alert persistence operations consumed by HealthChecker.
type healthAlertsRepo interface {
	UpsertAlert(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error)
	ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error)
}

// HealthChecker periodically polls service states and host metrics,
// generating alerts on failures and auto-resolving on recovery.
type HealthChecker struct {
	manager    *Manager
	alerts     healthAlertsRepo
	publish    HealthPublisher
	interval   time.Duration
	thresholds AlertThresholds

	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	doneCh    chan struct{}
}

// NewHealthChecker creates a health checker. If thresholds is zero-valued,
// defaults of 90/90/95 are applied.
func NewHealthChecker(mgr *Manager, alertsRepo healthAlertsRepo, publish HealthPublisher, interval time.Duration, thresholds AlertThresholds) *HealthChecker {
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	if thresholds.CPUPercent <= 0 {
		thresholds.CPUPercent = 90.0
	}
	if thresholds.MemPercent <= 0 {
		thresholds.MemPercent = 90.0
	}
	if thresholds.DiskPercent <= 0 {
		thresholds.DiskPercent = 95.0
	}
	return &HealthChecker{
		manager:    mgr,
		alerts:     alertsRepo,
		publish:    publish,
		interval:   interval,
		thresholds: thresholds,
		doneCh:     make(chan struct{}),
	}
}

// Start begins the periodic health check loop.
func (hc *HealthChecker) Start(ctx context.Context) {
	hc.startOnce.Do(func() {
		childCtx, cancel := context.WithCancel(ctx)
		hc.stopFn = cancel
		go hc.loop(childCtx)
	})
}

// Stop gracefully stops the health checker. It accepts a context for
// timeout control so it does not block shutdown indefinitely.
func (hc *HealthChecker) Stop(ctx context.Context) {
	hc.stopOnce.Do(func() {
		if hc.stopFn != nil {
			hc.stopFn()
		}
		select {
		case <-hc.doneCh:
		case <-ctx.Done():
		}
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
	svcs, err := hc.manager.ListServices(ctx)
	if err != nil {
		slog.Warn("health check: list services failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, svc := range svcs {
		state := strings.ToLower(strings.TrimSpace(svc.ActiveState))
		dedupeKey := fmt.Sprintf("health:service:%s:failed", svc.Name)

		switch state {
		case "failed":
			hc.raiseAlert(ctx, alerts.AlertWrite{
				DedupeKey: dedupeKey,
				Source:    "health",
				Resource:  svc.Name,
				Title:     fmt.Sprintf("Service %s failed", svc.DisplayName),
				Message:   fmt.Sprintf("Service %s is in failed state (unit=%s)", svc.DisplayName, svc.Unit),
				Severity:  "error",
				Metadata:  marshalMetadata(map[string]string{"service": svc.Name, "unit": svc.Unit, "state": state}),
				CreatedAt: now,
			})
		case stateActive, stateRunning:
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

	if metrics.CPUPercent > hc.thresholds.CPUPercent && metrics.CPUPercent >= 0 {
		hc.raiseAlert(ctx, alerts.AlertWrite{
			DedupeKey: "health:host:cpu:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High CPU usage",
			Message:   fmt.Sprintf("CPU usage is %.1f%% (threshold: %.0f%%)", metrics.CPUPercent, hc.thresholds.CPUPercent),
			Severity:  "warn",
			Metadata:  marshalMetadata(map[string]any{"cpuPercent": metrics.CPUPercent}),
			CreatedAt: now,
		})
	} else if metrics.CPUPercent >= 0 {
		hc.resolveAlert(ctx, "health:host:cpu:high", now)
	}

	if metrics.MemPercent > hc.thresholds.MemPercent {
		hc.raiseAlert(ctx, alerts.AlertWrite{
			DedupeKey: "health:host:memory:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High memory usage",
			Message:   fmt.Sprintf("Memory usage is %.1f%% (threshold: %.0f%%)", metrics.MemPercent, hc.thresholds.MemPercent),
			Severity:  "warn",
			Metadata:  marshalMetadata(map[string]any{"memPercent": metrics.MemPercent}),
			CreatedAt: now,
		})
	} else {
		hc.resolveAlert(ctx, "health:host:memory:high", now)
	}

	if metrics.DiskPercent > hc.thresholds.DiskPercent {
		hc.raiseAlert(ctx, alerts.AlertWrite{
			DedupeKey: "health:host:disk:high",
			Source:    "health",
			Resource:  "host",
			Title:     "High disk usage",
			Message:   fmt.Sprintf("Disk usage is %.1f%% (threshold: %.0f%%)", metrics.DiskPercent, hc.thresholds.DiskPercent),
			Severity:  "error",
			Metadata:  marshalMetadata(map[string]any{"diskPercent": metrics.DiskPercent}),
			CreatedAt: now,
		})
	} else {
		hc.resolveAlert(ctx, "health:host:disk:high", now)
	}
}

func (hc *HealthChecker) raiseAlert(ctx context.Context, write alerts.AlertWrite) {
	if hc.alerts == nil {
		return
	}
	alert, err := hc.alerts.UpsertAlert(ctx, write)
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
	if hc.alerts == nil {
		return
	}
	alert, err := hc.alerts.ResolveAlert(ctx, dedupeKey, at)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("health check: resolve alert failed", "dedupeKey", dedupeKey, "error", err)
		}
		return
	}
	if hc.publish != nil {
		hc.publish("ops.alerts.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"alert":     alert,
		})
	}
}

func marshalMetadata(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("failed to marshal metadata", "error", err)
		return "{}"
	}
	return string(b)
}
