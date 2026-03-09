package report

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/validate"
)

// SystemMetrics is a snapshot of host resource metrics included in the report.
type SystemMetrics struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemUsedBytes   int64   `json:"memUsedBytes"`
	MemTotalBytes  int64   `json:"memTotalBytes"`
	MemPercent     float64 `json:"memPercent"`
	DiskUsedBytes  int64   `json:"diskUsedBytes"`
	DiskTotalBytes int64   `json:"diskTotalBytes"`
	DiskPercent    float64 `json:"diskPercent"`
	LoadAvg1       float64 `json:"loadAvg1"`
	LoadAvg5       float64 `json:"loadAvg5"`
	LoadAvg15      float64 `json:"loadAvg15"`
}

// AlertSummary counts alerts by status.
type AlertSummary struct {
	Open     int `json:"open"`
	Acked    int `json:"acked"`
	Resolved int `json:"resolved"`
}

// EventSummary groups timeline event counts by source.
type EventSummary struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// ServiceStat captures the status of a tracked service.
type ServiceStat struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	ActiveState  string `json:"activeState"`
	EnabledState string `json:"enabledState"`
}

// HealthReport is the periodic health report payload sent via webhook.
type HealthReport struct {
	Event         string         `json:"event"`
	Host          string         `json:"host"`
	GeneratedAt   time.Time      `json:"generatedAt"`
	Metrics       SystemMetrics  `json:"metrics"`
	AlertSummary  AlertSummary   `json:"alertSummary"`
	RecentEvents  []EventSummary `json:"recentEvents"`
	ServiceStatus []ServiceStat  `json:"serviceStatus"`
}

// reportStore defines the data-fetching operations consumed by Generator.
type reportStore interface {
	ListAlerts(ctx context.Context, limit int, status string) ([]alerts.Alert, error)
	CountActivityEventsBySource(ctx context.Context, since time.Time) (map[string]int, error)
}

// metricsCollector abstracts system metrics collection.
type metricsCollector interface {
	Metrics(ctx context.Context) services.HostMetrics
	ListServices(ctx context.Context) ([]services.ServiceStatus, error)
}

// Generator produces health reports and delivers them via webhook.
// A nil *Generator is safe — all methods are no-ops.
type Generator struct {
	store    reportStore
	metrics  metricsCollector
	notifier *notify.Notifier

	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	doneCh    chan struct{}
}

// New creates a Generator. If notifier is nil the generator can still produce
// reports but GenerateAndSend will be a no-op.
func New(store reportStore, metrics metricsCollector, notifier *notify.Notifier) *Generator {
	return &Generator{
		store:    store,
		metrics:  metrics,
		notifier: notifier,
		doneCh:   make(chan struct{}),
	}
}

// Generate collects data and returns a HealthReport snapshot.
// Safe to call on a nil receiver (returns an empty report).
func (g *Generator) Generate(ctx context.Context) (*HealthReport, error) {
	if g == nil {
		return &HealthReport{}, nil
	}

	report := &HealthReport{
		Event:       "health.report",
		Host:        hostname(),
		GeneratedAt: time.Now().UTC(),
	}

	// Collect system metrics.
	if g.metrics != nil {
		m := g.metrics.Metrics(ctx)
		report.Metrics = SystemMetrics{
			CPUPercent:     m.CPUPercent,
			MemUsedBytes:   m.MemUsedBytes,
			MemTotalBytes:  m.MemTotalBytes,
			MemPercent:     m.MemPercent,
			DiskUsedBytes:  m.DiskUsedBytes,
			DiskTotalBytes: m.DiskTotalBytes,
			DiskPercent:    m.DiskPercent,
			LoadAvg1:       m.LoadAvg1,
			LoadAvg5:       m.LoadAvg5,
			LoadAvg15:      m.LoadAvg15,
		}
	}

	// Collect alert counts by status.
	if g.store != nil {
		for _, status := range []string{alerts.StatusOpen, alerts.StatusAcked, alerts.StatusResolved} {
			items, err := g.store.ListAlerts(ctx, 500, status)
			if err != nil {
				slog.Warn("health report: list alerts failed", "status", status, "error", err)
				continue
			}
			switch status {
			case alerts.StatusOpen:
				report.AlertSummary.Open = len(items)
			case alerts.StatusAcked:
				report.AlertSummary.Acked = len(items)
			case alerts.StatusResolved:
				report.AlertSummary.Resolved = len(items)
			}
		}

		// Collect timeline event counts by source (last 24h).
		since := time.Now().UTC().Add(-24 * time.Hour)
		counts, err := g.store.CountActivityEventsBySource(ctx, since)
		if err != nil {
			slog.Warn("health report: count activity events failed", "error", err)
		} else {
			for source, count := range counts {
				report.RecentEvents = append(report.RecentEvents, EventSummary{
					Source: source,
					Count:  count,
				})
			}
		}
	}

	// Collect service statuses.
	if g.metrics != nil {
		svcs, err := g.metrics.ListServices(ctx)
		if err != nil {
			slog.Warn("health report: list services failed", "error", err)
		} else {
			for _, svc := range svcs {
				report.ServiceStatus = append(report.ServiceStatus, ServiceStat{
					Name:         svc.Name,
					DisplayName:  svc.DisplayName,
					ActiveState:  svc.ActiveState,
					EnabledState: svc.EnabledState,
				})
			}
		}
	}

	return report, nil
}

// GenerateAndSend generates a report and sends it via webhook.
// Safe to call on a nil receiver.
func (g *Generator) GenerateAndSend(ctx context.Context) error {
	if g == nil {
		return nil
	}

	report, err := g.Generate(ctx)
	if err != nil {
		return fmt.Errorf("generate health report: %w", err)
	}

	if err := g.notifier.SendJSON(ctx, report); err != nil {
		return fmt.Errorf("send health report: %w", err)
	}

	slog.Info("health report sent", "host", report.Host, "generatedAt", report.GeneratedAt)
	return nil
}

// StartSchedule begins a cron-based loop that calls GenerateAndSend at
// the times specified by cronExpr. The timezone parameter controls the
// schedule evaluation location (IANA name, e.g. "America/Sao_Paulo").
// Safe to call on a nil receiver.
func (g *Generator) StartSchedule(parent context.Context, cronExpr, timezone string) error {
	if g == nil {
		return nil
	}

	sched, err := validate.ParseCron(cronExpr)
	if err != nil {
		return fmt.Errorf("parse health report schedule: %w", err)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	g.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		g.stopFn = cancel

		go func() {
			defer close(g.doneCh)
			for {
				now := time.Now().In(loc)
				next := sched.Next(now)
				delay := time.Until(next)

				slog.Info("health report scheduled", "next", next.Format(time.RFC3339), "delay", delay.Truncate(time.Second))

				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}

				sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Second)
				if err := g.GenerateAndSend(sendCtx); err != nil {
					slog.Warn("health report delivery failed", "error", err)
				}
				sendCancel()
			}
		}()
	})

	return nil
}

// Stop gracefully stops the scheduled report loop. Accepts a context for
// timeout control so it does not block shutdown indefinitely.
// Safe to call on a nil receiver.
func (g *Generator) Stop(ctx context.Context) {
	if g == nil {
		return
	}
	g.stopOnce.Do(func() {
		if g.stopFn != nil {
			g.stopFn()
		}
		select {
		case <-g.doneCh:
		case <-ctx.Done():
		}
	})
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
