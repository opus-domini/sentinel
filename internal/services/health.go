package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/updater"
)

const (
	resourceHost    = "host"
	resourceUpdater = "updater"
	keyAlert        = "alert"
)

const defaultHealthInterval = 30 * time.Second

// updaterStaleThreshold bounds how old an autoupdate failure may be before it
// is treated as stale (e.g. autoupdate disabled) and no longer alerted on.
const updaterStaleThreshold = 48 * time.Hour

// alertSourceHealth tags alerts raised by the service health checker.
const alertSourceHealth = "health"

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

// healthActivityRepo is an optional interface for recording timeline events.
// When non-nil, alert lifecycle events are written to the ops timeline.
type healthActivityRepo interface {
	InsertActivityEvent(ctx context.Context, write activity.EventWrite) (activity.Event, error)
}

// HealthChecker periodically polls service states and host metrics,
// generating alerts on failures and auto-resolving on recovery.
type HealthChecker struct {
	manager    *Manager
	alerts     healthAlertsRepo
	activity   healthActivityRepo
	notifier   *notify.Notifier
	publish    HealthPublisher
	interval   time.Duration
	thresholds AlertThresholds

	updaterStateDir string

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

// SetActivityRepo sets an optional activity repository for recording
// alert lifecycle events in the ops timeline. Must be called before Start.
func (hc *HealthChecker) SetActivityRepo(repo healthActivityRepo) {
	hc.activity = repo
}

// SetNotifier sets an optional webhook notifier for alert events.
// Must be called before Start.
func (hc *HealthChecker) SetNotifier(n *notify.Notifier) {
	hc.notifier = n
}

// SetUpdaterStateDir enables autoupdate health monitoring. dir is the Sentinel
// data directory; the updater state is read from <dir>/updater/state.json.
// Must be called before Start.
func (hc *HealthChecker) SetUpdaterStateDir(dir string) {
	hc.updaterStateDir = strings.TrimSpace(dir)
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
	hc.checkUpdater(ctx)
}

// checkUpdater raises an alert when the most recent automatic update failed,
// and resolves it once a later run succeeds. Stale failures (older than
// updaterStaleThreshold) are ignored so a disabled updater stops nagging.
func (hc *HealthChecker) checkUpdater(ctx context.Context) {
	if hc.updaterStateDir == "" {
		return
	}
	state, err := updater.Status(hc.updaterStateDir)
	if err != nil {
		slog.Warn("health check: read updater state failed", "error", err)
		return
	}

	now := time.Now().UTC()
	const dedupeKey = "health:updater:failed"
	lastError := strings.TrimSpace(state.LastError)
	if lastError == "" || state.LastCheckedAt.IsZero() ||
		now.Sub(state.LastCheckedAt.UTC()) > updaterStaleThreshold {
		hc.resolveAlert(ctx, dedupeKey, now)
		return
	}

	hc.raiseAlert(ctx, alerts.AlertWrite{
		DedupeKey: dedupeKey,
		Source:    alertSourceHealth,
		Resource:  resourceUpdater,
		Title:     "Automatic update failed",
		Message:   fmt.Sprintf("The last automatic update attempt failed: %s", lastError),
		Severity:  activity.SeverityWarn,
		Metadata: marshalMetadata(map[string]string{
			"currentVersion": state.CurrentVersion,
			"latestVersion":  state.LatestVersion,
		}),
		CreatedAt: now,
	})
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
				Source:    alertSourceHealth,
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
			Source:    alertSourceHealth,
			Resource:  resourceHost,
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
			Source:    alertSourceHealth,
			Resource:  resourceHost,
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
			Source:    alertSourceHealth,
			Resource:  resourceHost,
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
	if hc.activity != nil {
		if _, teErr := hc.activity.InsertActivityEvent(ctx, activity.EventWrite{
			Source:    activity.SourceAlert,
			EventType: "alert.created",
			Severity:  activity.NormalizeSeverity(alert.Severity),
			Resource:  alert.Resource,
			Message:   fmt.Sprintf("Alert created: %s", alert.Title),
			Details:   alert.Message,
			Metadata:  marshalMetadata(map[string]any{"alertId": alert.ID, "dedupeKey": alert.DedupeKey, "source": alertSourceHealth}),
			CreatedAt: write.CreatedAt,
		}); teErr != nil {
			slog.Warn("health check: record alert.created event failed", "error", teErr)
		}
	}
	hc.notifier.SendAsync(notify.AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     alert,
		Host:      hostname(),
		Timestamp: write.CreatedAt,
	})
	if hc.publish != nil {
		hc.publish("ops.alerts.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			keyAlert:    alert,
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
	if hc.activity != nil {
		if _, teErr := hc.activity.InsertActivityEvent(ctx, activity.EventWrite{
			Source:    activity.SourceAlert,
			EventType: "alert.resolved",
			Severity:  "info",
			Resource:  alert.Resource,
			Message:   fmt.Sprintf("Alert resolved: %s", alert.Title),
			Details:   alert.Message,
			Metadata:  marshalMetadata(map[string]any{"alertId": alert.ID, "dedupeKey": alert.DedupeKey, "source": alertSourceHealth}),
			CreatedAt: at,
		}); teErr != nil {
			slog.Warn("health check: record alert.resolved event failed", "error", teErr)
		}
	}
	hc.notifier.SendAsync(notify.AlertWebhookPayload{
		Event:     "alert.resolved",
		Alert:     alert,
		Host:      hostname(),
		Timestamp: at,
	})
	if hc.publish != nil {
		hc.publish("ops.alerts.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			keyAlert:    alert,
		})
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func marshalMetadata(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("failed to marshal metadata", "error", err)
		return "{}"
	}
	return string(b)
}
