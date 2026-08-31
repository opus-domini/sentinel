// Package report builds diagnostic reports.
package report

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/validate"
)

var osHostname = os.Hostname

// SystemMetrics is a snapshot of host resource metrics included in the report.
type SystemMetrics struct {
	CPUPercent     float64                `json:"cpuPercent"`
	MemUsedBytes   int64                  `json:"memUsedBytes"`
	MemTotalBytes  int64                  `json:"memTotalBytes"`
	MemPercent     float64                `json:"memPercent"`
	DiskUsedBytes  int64                  `json:"diskUsedBytes"`
	DiskTotalBytes int64                  `json:"diskTotalBytes"`
	DiskPercent    float64                `json:"diskPercent"`
	LoadAvg1       float64                `json:"loadAvg1"`
	LoadAvg5       float64                `json:"loadAvg5"`
	LoadAvg15      float64                `json:"loadAvg15"`
	Sensors        services.SensorMetrics `json:"sensors"`
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
	Event         string        `json:"event"`
	Host          string        `json:"host"`
	GeneratedAt   time.Time     `json:"generatedAt"`
	Metrics       SystemMetrics `json:"metrics"`
	ServiceStatus []ServiceStat `json:"serviceStatus"`
}

// metricsCollector abstracts system metrics collection.
type metricsCollector interface {
	MetricsSnapshot(ctx context.Context) services.MetricsSnapshot
	ListServices(ctx context.Context) ([]services.ServiceStatus, error)
}

// schedule is the subset of cron.Schedule the report loop needs: the next
// activation time strictly after the given time.
type schedule interface {
	Next(time.Time) time.Time
}

// Generator produces health reports and delivers them via webhook.
// A nil *Generator is safe — all methods are no-ops.
type Generator struct {
	metrics  metricsCollector
	notifier *notify.Notifier

	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	// doneCh is allocated by StartSchedule and closed when the loop exits, so
	// it stays nil while no loop has ever been started.
	doneCh chan struct{}
}

// New creates a Generator. If notifier is nil the generator can still produce
// reports but GenerateAndSend will be a no-op.
func New(metrics metricsCollector, notifier *notify.Notifier) *Generator {
	return &Generator{
		metrics:  metrics,
		notifier: notifier,
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
		m := g.metrics.MetricsSnapshot(ctx).Metrics
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
			Sensors:        m.Sensors,
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

	if g.notifier == nil {
		return nil
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

	// robfig/cron gives up after five years and returns the zero time for an
	// expression that never occurs (e.g. "0 0 30 2 *"). time.Until of the zero
	// time is a ~2000-year negative duration, so the loop's timer would fire
	// immediately and spin, sending a report per iteration. Refuse to start.
	if sched.Next(time.Now().In(loc)).IsZero() {
		return fmt.Errorf("health report schedule %q never fires", cronExpr)
	}

	g.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		g.stopFn = cancel
		g.doneCh = make(chan struct{})

		go func() {
			defer close(g.doneCh)
			g.runSchedule(ctx, sched, loc)
		}()
	})

	return nil
}

// runSchedule blocks until ctx is cancelled, sending a report at every time
// sched produces. A delivery failure is logged and the loop keeps running.
func (g *Generator) runSchedule(ctx context.Context, sched schedule, loc *time.Location) {
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
		// Never started (e.g. empty/invalid schedule): no loop to wait for, so
		// don't block the shutdown deadline on a nil channel.
		if g.doneCh == nil {
			return
		}
		select {
		case <-g.doneCh:
		case <-ctx.Done():
		}
	})
}

func hostname() string {
	h, err := osHostname()
	if err != nil {
		return "unknown"
	}
	return h
}
