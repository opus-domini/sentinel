package report

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/services"
)

// mockStore implements reportStore for testing.
type mockStore struct {
	listAlerts  func(ctx context.Context, limit int, status string) ([]alerts.Alert, error)
	countEvents func(ctx context.Context, since time.Time) (map[string]int, error)
}

func (m *mockStore) ListAlerts(ctx context.Context, limit int, status string) ([]alerts.Alert, error) {
	if m.listAlerts != nil {
		return m.listAlerts(ctx, limit, status)
	}
	return nil, nil
}

func (m *mockStore) CountActivityEventsBySource(ctx context.Context, since time.Time) (map[string]int, error) {
	if m.countEvents != nil {
		return m.countEvents(ctx, since)
	}
	return nil, nil
}

// mockMetrics implements metricsCollector for testing.
type mockMetrics struct {
	metrics      services.HostMetrics
	servicesList []services.ServiceStatus
	listErr      error
}

func (m *mockMetrics) Metrics(_ context.Context) services.HostMetrics {
	return m.metrics
}

func (m *mockMetrics) ListServices(_ context.Context) ([]services.ServiceStatus, error) {
	return m.servicesList, m.listErr
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		store            reportStore
		metrics          metricsCollector
		wantOpen         int
		wantAcked        int
		wantResolved     int
		wantEventCount   int
		wantServiceCount int
	}{
		{
			name: "full report with all data",
			store: &mockStore{
				listAlerts: func(_ context.Context, _ int, status string) ([]alerts.Alert, error) {
					switch status {
					case alerts.StatusOpen:
						return []alerts.Alert{{ID: 1}, {ID: 2}}, nil
					case alerts.StatusAcked:
						return []alerts.Alert{{ID: 3}}, nil
					case alerts.StatusResolved:
						return []alerts.Alert{{ID: 4}, {ID: 5}, {ID: 6}}, nil
					}
					return nil, nil
				},
				countEvents: func(_ context.Context, _ time.Time) (map[string]int, error) {
					return map[string]int{
						"alert":   5,
						"runbook": 3,
					}, nil
				},
			},
			metrics: &mockMetrics{
				metrics: services.HostMetrics{
					CPUPercent:     45.2,
					MemPercent:     62.1,
					MemUsedBytes:   4 * 1024 * 1024 * 1024,
					MemTotalBytes:  8 * 1024 * 1024 * 1024,
					DiskPercent:    78.5,
					DiskUsedBytes:  200 * 1024 * 1024 * 1024,
					DiskTotalBytes: 256 * 1024 * 1024 * 1024,
					LoadAvg1:       1.5,
					LoadAvg5:       1.2,
					LoadAvg15:      0.9,
				},
				servicesList: []services.ServiceStatus{
					{Name: "sentinel", DisplayName: "Sentinel service", ActiveState: "active", EnabledState: "enabled"},
					{Name: "sentinel-updater", DisplayName: "Autoupdate timer", ActiveState: "active", EnabledState: "enabled"},
				},
			},
			wantOpen:         2,
			wantAcked:        1,
			wantResolved:     3,
			wantEventCount:   2,
			wantServiceCount: 2,
		},
		{
			name:             "nil store and nil metrics",
			store:            nil,
			metrics:          nil,
			wantOpen:         0,
			wantAcked:        0,
			wantResolved:     0,
			wantEventCount:   0,
			wantServiceCount: 0,
		},
		{
			name: "store only, no metrics",
			store: &mockStore{
				listAlerts: func(_ context.Context, _ int, status string) ([]alerts.Alert, error) {
					if status == alerts.StatusOpen {
						return []alerts.Alert{{ID: 1}}, nil
					}
					return nil, nil
				},
				countEvents: func(_ context.Context, _ time.Time) (map[string]int, error) {
					return map[string]int{"health": 10}, nil
				},
			},
			metrics:          nil,
			wantOpen:         1,
			wantAcked:        0,
			wantResolved:     0,
			wantEventCount:   1,
			wantServiceCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := New(tc.store, tc.metrics, nil)
			report, err := g.Generate(context.Background())
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}

			if report.Event != "health.report" {
				t.Errorf("Event = %q, want %q", report.Event, "health.report")
			}
			if report.GeneratedAt.IsZero() {
				t.Error("GeneratedAt is zero")
			}
			if report.AlertSummary.Open != tc.wantOpen {
				t.Errorf("AlertSummary.Open = %d, want %d", report.AlertSummary.Open, tc.wantOpen)
			}
			if report.AlertSummary.Acked != tc.wantAcked {
				t.Errorf("AlertSummary.Acked = %d, want %d", report.AlertSummary.Acked, tc.wantAcked)
			}
			if report.AlertSummary.Resolved != tc.wantResolved {
				t.Errorf("AlertSummary.Resolved = %d, want %d", report.AlertSummary.Resolved, tc.wantResolved)
			}
			if len(report.RecentEvents) != tc.wantEventCount {
				t.Errorf("RecentEvents count = %d, want %d", len(report.RecentEvents), tc.wantEventCount)
			}
			if len(report.ServiceStatus) != tc.wantServiceCount {
				t.Errorf("ServiceStatus count = %d, want %d", len(report.ServiceStatus), tc.wantServiceCount)
			}
		})
	}
}

func TestGenerateMetricsPopulated(t *testing.T) {
	t.Parallel()

	g := New(nil, &mockMetrics{
		metrics: services.HostMetrics{
			CPUPercent: 55.5,
			MemPercent: 70.0,
			LoadAvg1:   2.0,
		},
	}, nil)

	report, err := g.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if report.Metrics.CPUPercent != 55.5 {
		t.Errorf("Metrics.CPUPercent = %f, want 55.5", report.Metrics.CPUPercent)
	}
	if report.Metrics.MemPercent != 70.0 {
		t.Errorf("Metrics.MemPercent = %f, want 70.0", report.Metrics.MemPercent)
	}
	if report.Metrics.LoadAvg1 != 2.0 {
		t.Errorf("Metrics.LoadAvg1 = %f, want 2.0", report.Metrics.LoadAvg1)
	}
}

func TestHealthReportJSONSerialization(t *testing.T) {
	t.Parallel()

	report := HealthReport{
		Event:       "health.report",
		Host:        "web-01",
		GeneratedAt: time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC),
		Metrics: SystemMetrics{
			CPUPercent:     42.5,
			MemPercent:     65.0,
			MemUsedBytes:   4294967296,
			MemTotalBytes:  8589934592,
			DiskPercent:    80.0,
			DiskUsedBytes:  214748364800,
			DiskTotalBytes: 268435456000,
			LoadAvg1:       1.2,
			LoadAvg5:       1.0,
			LoadAvg15:      0.8,
		},
		AlertSummary: AlertSummary{
			Open:     2,
			Acked:    1,
			Resolved: 5,
		},
		RecentEvents: []EventSummary{
			{Source: "alert", Count: 10},
			{Source: "runbook", Count: 3},
		},
		ServiceStatus: []ServiceStat{
			{Name: "sentinel", DisplayName: "Sentinel service", ActiveState: "active", EnabledState: "enabled"},
		},
	}

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded["event"] != "health.report" {
		t.Errorf("event = %v, want health.report", decoded["event"])
	}
	if decoded["host"] != "web-01" {
		t.Errorf("host = %v, want web-01", decoded["host"])
	}

	metrics, ok := decoded["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics is not a map: %T", decoded["metrics"])
	}
	if metrics["cpuPercent"] != 42.5 {
		t.Errorf("metrics.cpuPercent = %v, want 42.5", metrics["cpuPercent"])
	}

	alertSummary, ok := decoded["alertSummary"].(map[string]any)
	if !ok {
		t.Fatalf("alertSummary is not a map: %T", decoded["alertSummary"])
	}
	if alertSummary["open"] != float64(2) {
		t.Errorf("alertSummary.open = %v, want 2", alertSummary["open"])
	}

	events, ok := decoded["recentEvents"].([]any)
	if !ok {
		t.Fatalf("recentEvents is not an array: %T", decoded["recentEvents"])
	}
	if len(events) != 2 {
		t.Errorf("recentEvents length = %d, want 2", len(events))
	}

	svcs, ok := decoded["serviceStatus"].([]any)
	if !ok {
		t.Fatalf("serviceStatus is not an array: %T", decoded["serviceStatus"])
	}
	if len(svcs) != 1 {
		t.Errorf("serviceStatus length = %d, want 1", len(svcs))
	}
}

func TestNilGeneratorIsSafe(t *testing.T) {
	t.Parallel()

	var g *Generator

	// Generate on nil should return empty report without error.
	report, err := g.Generate(context.Background())
	if err != nil {
		t.Fatalf("nil Generate() error: %v", err)
	}
	if report == nil {
		t.Fatal("nil Generate() returned nil report")
	}

	// GenerateAndSend on nil should be a no-op.
	if err := g.GenerateAndSend(context.Background()); err != nil {
		t.Fatalf("nil GenerateAndSend() error: %v", err)
	}

	// Stop on nil should not panic.
	g.Stop(context.Background())
}

func TestGenerateAndSendWithNilNotifier(t *testing.T) {
	t.Parallel()

	g := New(&mockStore{}, &mockMetrics{}, nil)
	err := g.GenerateAndSend(context.Background())
	if err != nil {
		t.Fatalf("GenerateAndSend() with nil notifier error: %v", err)
	}
}

func TestStartScheduleInvalidCron(t *testing.T) {
	t.Parallel()

	g := New(&mockStore{}, &mockMetrics{}, nil)
	err := g.StartSchedule(context.Background(), "not-a-cron", "UTC")
	if err == nil {
		t.Fatal("StartSchedule() with invalid cron should return error")
	}
}

func TestStopWithoutStart(t *testing.T) {
	t.Parallel()

	g := New(&mockStore{}, &mockMetrics{}, nil)
	// Stop without Start should not panic or block.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	g.Stop(ctx)
}
