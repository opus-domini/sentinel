package report

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/services"
)

// mockMetrics implements metricsCollector for testing.
type mockMetrics struct {
	metrics      services.HostMetrics
	servicesList []services.ServiceStatus
	listErr      error
	// collections, when set, receives one value per MetricsSnapshot call so
	// loop tests can count scheduled collections.
	collections chan struct{}
}

func (m *mockMetrics) MetricsSnapshot(_ context.Context) services.MetricsSnapshot {
	if m.collections != nil {
		select {
		case m.collections <- struct{}{}:
		default:
		}
	}
	return services.MetricsSnapshot{Metrics: m.metrics}
}

func (m *mockMetrics) ListServices(_ context.Context) ([]services.ServiceStatus, error) {
	return m.servicesList, m.listErr
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		metrics          metricsCollector
		wantServiceCount int
	}{
		{
			name: "full report with metrics and services",
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
			wantServiceCount: 2,
		},
		{
			name:             "nil metrics",
			metrics:          nil,
			wantServiceCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := New(tc.metrics, nil)
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
			if len(report.ServiceStatus) != tc.wantServiceCount {
				t.Errorf("ServiceStatus count = %d, want %d", len(report.ServiceStatus), tc.wantServiceCount)
			}
		})
	}
}

func TestGenerateMetricsPopulated(t *testing.T) {
	t.Parallel()

	g := New(&mockMetrics{
		metrics: services.HostMetrics{
			CPUPercent: 55.5,
			MemPercent: 70.0,
			LoadAvg1:   2.0,
			Sensors: services.SensorMetrics{
				Temperatures: []services.TemperatureSensor{
					{ID: "hwmon0:temp1", Label: "Package", Source: "coretemp", Celsius: 62.5},
				},
				Fans:  []services.FanSensor{},
				Power: []services.PowerSensor{},
			},
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
	if len(report.Metrics.Sensors.Temperatures) != 1 ||
		report.Metrics.Sensors.Temperatures[0].Celsius != 62.5 {
		t.Errorf("Metrics.Sensors = %+v", report.Metrics.Sensors)
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

	g := New(&mockMetrics{}, nil)
	err := g.GenerateAndSend(context.Background())
	if err != nil {
		t.Fatalf("GenerateAndSend() with nil notifier error: %v", err)
	}
}

func TestStartScheduleInvalidCron(t *testing.T) {
	t.Parallel()

	g := New(&mockMetrics{}, nil)
	err := g.StartSchedule(context.Background(), "not-a-cron", "UTC")
	if err == nil {
		t.Fatal("StartSchedule() with invalid cron should return error")
	}
}

func TestStopWithoutStart(t *testing.T) {
	t.Parallel()

	g := New(&mockMetrics{}, nil)
	// Stop without Start has no loop to wait for, so it must return right
	// away instead of burning the caller's whole shutdown budget.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	g.Stop(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Stop() without Start took %v, want immediate return", elapsed)
	}
}

func TestStopAfterStart(t *testing.T) {
	t.Parallel()

	g := New(&mockMetrics{}, nil)
	if err := g.StartSchedule(context.Background(), "0 3 * * *", "UTC"); err != nil {
		t.Fatalf("StartSchedule() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	g.Stop(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Stop() after Start took %v, want the loop to exit well before the deadline", elapsed)
	}

	select {
	case <-g.doneCh:
	default:
		t.Fatal("Stop() returned before the schedule loop closed doneCh")
	}
}

func TestRunScheduleDeliversReport(t *testing.T) {
	t.Parallel()

	payloads := make(chan HealthReport, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got HealthReport
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode webhook payload: %v", err)
		}
		select {
		case payloads <- got:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	g := New(&mockMetrics{
		metrics: services.HostMetrics{CPUPercent: 12.5},
		servicesList: []services.ServiceStatus{
			{Name: "sentinel", DisplayName: "Sentinel service", ActiveState: "active", EnabledState: "enabled"},
		},
	}, notify.New(srv.URL))

	done := runScheduleInBackground(t, g)

	select {
	case got := <-payloads:
		if got.Event != "health.report" {
			t.Errorf("Event = %q, want %q", got.Event, "health.report")
		}
		if got.Host != "sentinel-test-host" {
			t.Errorf("Host = %q, want %q", got.Host, "sentinel-test-host")
		}
		if got.GeneratedAt.IsZero() {
			t.Error("GeneratedAt is zero")
		}
		if got.Metrics.CPUPercent != 12.5 {
			t.Errorf("Metrics.CPUPercent = %f, want 12.5", got.Metrics.CPUPercent)
		}
		if len(got.ServiceStatus) != 1 {
			t.Errorf("ServiceStatus count = %d, want 1", len(got.ServiceStatus))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no health report delivered")
	}

	done()
}

func TestRunScheduleSurvivesDeliveryFailure(t *testing.T) {
	t.Parallel()

	// Buffered so the handler never blocks once the test has seen enough.
	hits := make(chan struct{}, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case hits <- struct{}{}:
		default:
		}
		// 4xx is not retried by the notifier, so each tick fails immediately.
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	// Counting collections rather than webhook hits keeps the assertion about
	// the loop surviving, independent of the notifier's retry policy.
	collections := make(chan struct{}, 8)
	g := New(&mockMetrics{collections: collections}, notify.New(srv.URL))

	done := runScheduleInBackground(t, g)

	// A second tick proves the loop outlived the first failed delivery.
	for range 2 {
		select {
		case <-collections:
		case <-time.After(10 * time.Second):
			t.Fatal("schedule loop stopped after a failed delivery")
		}
	}

	select {
	case <-hits:
	default:
		t.Error("webhook was never called, so no delivery failure was exercised")
	}

	done()
}

// fakeSchedule fires every interval, so loop tests do not wait on wall-clock
// cron boundaries.
type fakeSchedule struct{ interval time.Duration }

func (f fakeSchedule) Next(t time.Time) time.Time { return t.Add(f.interval) }

// runScheduleInBackground starts g.runSchedule on a fast fake schedule and
// returns a func that cancels it and waits for the loop to exit.
func runScheduleInBackground(t *testing.T, g *Generator) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.runSchedule(ctx, fakeSchedule{interval: time.Millisecond}, time.UTC)
	}()

	return func() {
		t.Helper()
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("runSchedule did not return after cancellation")
		}
	}
}
