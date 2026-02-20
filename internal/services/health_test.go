package services

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/daemon"
)

const testHealthHost = "test"

type stubAlertsRepo struct {
	mu        sync.Mutex
	upserted  []alerts.AlertWrite
	resolved  []string
	upsertFn  func(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error)
	resolveFn func(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error)
}

func (s *stubAlertsRepo) UpsertAlert(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error) {
	s.mu.Lock()
	s.upserted = append(s.upserted, write)
	s.mu.Unlock()
	if s.upsertFn != nil {
		return s.upsertFn(ctx, write)
	}
	return alerts.Alert{ID: 1, DedupeKey: write.DedupeKey, Status: "active"}, nil
}

func (s *stubAlertsRepo) ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error) {
	s.mu.Lock()
	s.resolved = append(s.resolved, dedupeKey)
	s.mu.Unlock()
	if s.resolveFn != nil {
		return s.resolveFn(ctx, dedupeKey, at)
	}
	return alerts.Alert{ID: 1, DedupeKey: dedupeKey, Status: "resolved"}, nil
}

func TestNewHealthChecker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		interval     time.Duration
		thresholds   AlertThresholds
		wantInterval time.Duration
		wantCPU      float64
		wantMem      float64
		wantDisk     float64
	}{
		{
			name:         "defaults applied for zero values",
			interval:     0,
			thresholds:   AlertThresholds{},
			wantInterval: defaultHealthInterval,
			wantCPU:      90.0,
			wantMem:      90.0,
			wantDisk:     95.0,
		},
		{
			name:         "negative interval uses default",
			interval:     -5 * time.Second,
			thresholds:   AlertThresholds{CPUPercent: 80, MemPercent: 85, DiskPercent: 90},
			wantInterval: defaultHealthInterval,
			wantCPU:      80.0,
			wantMem:      85.0,
			wantDisk:     90.0,
		},
		{
			name:         "custom values preserved",
			interval:     10 * time.Second,
			thresholds:   AlertThresholds{CPUPercent: 70, MemPercent: 75, DiskPercent: 80},
			wantInterval: 10 * time.Second,
			wantCPU:      70.0,
			wantMem:      75.0,
			wantDisk:     80.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hc := NewHealthChecker(nil, nil, nil, tc.interval, tc.thresholds)
			if hc.interval != tc.wantInterval {
				t.Fatalf("interval = %v, want %v", hc.interval, tc.wantInterval)
			}
			if hc.thresholds.CPUPercent != tc.wantCPU {
				t.Fatalf("CPUPercent = %v, want %v", hc.thresholds.CPUPercent, tc.wantCPU)
			}
			if hc.thresholds.MemPercent != tc.wantMem {
				t.Fatalf("MemPercent = %v, want %v", hc.thresholds.MemPercent, tc.wantMem)
			}
			if hc.thresholds.DiskPercent != tc.wantDisk {
				t.Fatalf("DiskPercent = %v, want %v", hc.thresholds.DiskPercent, tc.wantDisk)
			}
			if hc.doneCh == nil {
				t.Fatal("doneCh not initialized")
			}
		})
	}
}

func TestHealthCheckerStartStop(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		nowFn:    time.Now,
		hostname: func() (string, error) { return testHealthHost, nil },
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{ActiveState: "active", EnabledState: "enabled", UnitFileExists: true, ServicePath: "/home/dev/.config/systemd/user/sentinel.service"}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{TimerActiveState: "active", TimerEnabledState: "enabled", TimerUnitExists: true}, nil
		},
	}
	repo := &stubAlertsRepo{}
	hc := NewHealthChecker(mgr, repo, nil, 10*time.Millisecond, AlertThresholds{CPUPercent: 99, MemPercent: 99, DiskPercent: 99})

	ctx := context.Background()
	hc.Start(ctx)
	// Starting twice should be a no-op (sync.Once).
	hc.Start(ctx)

	// Let at least one tick fire.
	time.Sleep(50 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	hc.Stop(stopCtx)
	// Stopping twice should be safe (sync.Once).
	hc.Stop(stopCtx)
}

func TestCheckServicesRaisesAlertOnFailedService(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	mgr := &Manager{
		nowFn:    time.Now,
		hostname: func() (string, error) { return testHealthHost, nil },
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "failed",
			}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{
				TimerUnitExists:   true,
				TimerActiveState:  "active",
				TimerEnabledState: "enabled",
			}, nil
		},
	}

	hc := &HealthChecker{
		manager:    mgr,
		alerts:     repo,
		thresholds: AlertThresholds{CPUPercent: 99, MemPercent: 99, DiskPercent: 99},
	}

	hc.checkServices(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.upserted) == 0 {
		t.Fatal("expected at least one alert upserted for failed service")
	}
	found := false
	for _, a := range repo.upserted {
		if a.DedupeKey == "health:service:sentinel:failed" {
			found = true
			if a.Severity != "error" {
				t.Fatalf("severity = %q, want error", a.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected alert with dedupeKey health:service:sentinel:failed")
	}
}

func TestCheckServicesResolvesOnActive(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	mgr := &Manager{
		nowFn:    time.Now,
		hostname: func() (string, error) { return testHealthHost, nil },
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{
				TimerUnitExists:   true,
				TimerActiveState:  "running",
				TimerEnabledState: "enabled",
			}, nil
		},
	}

	hc := &HealthChecker{
		manager:    mgr,
		alerts:     repo,
		thresholds: AlertThresholds{CPUPercent: 99, MemPercent: 99, DiskPercent: 99},
	}

	hc.checkServices(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.resolved) < 2 {
		t.Fatalf("expected at least 2 resolved alerts (sentinel + updater), got %d", len(repo.resolved))
	}
}

func TestCheckServicesNilManager(t *testing.T) {
	t.Parallel()

	hc := &HealthChecker{manager: nil}
	// Should not panic.
	hc.checkServices(context.Background())
}

func TestCheckMetricsNilManager(t *testing.T) {
	t.Parallel()

	hc := &HealthChecker{manager: nil}
	// Should not panic.
	hc.checkMetrics(context.Background())
}

func TestCheckServicesListError(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	mgr := &Manager{
		nowFn: time.Now,
		goos:  "linux",
		uidFn: func() int { return 1000 },
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{}, errors.New("daemon unavailable")
		},
	}

	hc := &HealthChecker{
		manager:    mgr,
		alerts:     repo,
		thresholds: AlertThresholds{CPUPercent: 99, MemPercent: 99, DiskPercent: 99},
	}

	// Should not panic, just log warning.
	hc.checkServices(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.upserted) != 0 {
		t.Fatalf("expected no alerts on list error, got %d", len(repo.upserted))
	}
}

func TestRaiseAlertNilRepo(t *testing.T) {
	t.Parallel()

	hc := &HealthChecker{alerts: nil}
	// Should not panic.
	hc.raiseAlert(context.Background(), alerts.AlertWrite{DedupeKey: "test"})
}

func TestRaiseAlertUpsertError(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{
		upsertFn: func(_ context.Context, _ alerts.AlertWrite) (alerts.Alert, error) {
			return alerts.Alert{}, errors.New("db error")
		},
	}
	hc := &HealthChecker{alerts: repo}
	// Should not panic, just log warning.
	hc.raiseAlert(context.Background(), alerts.AlertWrite{DedupeKey: "test"})
}

func TestRaiseAlertPublishes(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	var published bool
	hc := &HealthChecker{
		alerts: repo,
		publish: func(eventType string, payload map[string]any) {
			published = true
			if eventType != "ops.alerts.updated" {
				t.Errorf("eventType = %q, want ops.alerts.updated", eventType)
			}
		},
	}

	hc.raiseAlert(context.Background(), alerts.AlertWrite{DedupeKey: "test"})
	if !published {
		t.Fatal("expected publish to be called")
	}
}

func TestResolveAlertNilRepo(t *testing.T) {
	t.Parallel()

	hc := &HealthChecker{alerts: nil}
	// Should not panic.
	hc.resolveAlert(context.Background(), "test", time.Now())
}

func TestResolveAlertNoRowsError(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{
		resolveFn: func(_ context.Context, _ string, _ time.Time) (alerts.Alert, error) {
			return alerts.Alert{}, sql.ErrNoRows
		},
	}
	hc := &HealthChecker{alerts: repo}
	// Should not panic — sql.ErrNoRows is silently ignored.
	hc.resolveAlert(context.Background(), "test", time.Now())
}

func TestResolveAlertOtherError(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{
		resolveFn: func(_ context.Context, _ string, _ time.Time) (alerts.Alert, error) {
			return alerts.Alert{}, errors.New("unexpected error")
		},
	}
	hc := &HealthChecker{alerts: repo}
	// Should not panic — logs a warning.
	hc.resolveAlert(context.Background(), "test", time.Now())
}

func TestResolveAlertPublishes(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	var published bool
	hc := &HealthChecker{
		alerts: repo,
		publish: func(eventType string, payload map[string]any) {
			published = true
			if eventType != "ops.alerts.updated" {
				t.Errorf("eventType = %q, want ops.alerts.updated", eventType)
			}
		},
	}

	hc.resolveAlert(context.Background(), "test", time.Now())
	if !published {
		t.Fatal("expected publish to be called")
	}
}

func TestMarshalMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:  "map of strings",
			input: map[string]string{"key": "value"},
			want:  `{"key":"value"}`,
		},
		{
			name:  "map of any",
			input: map[string]any{"cpu": 95.5},
			want:  `{"cpu":95.5}`,
		},
		{
			name:    "unmarshallable value returns empty object",
			input:   func() {},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := marshalMetadata(tc.input)
			if tc.wantErr {
				if got != "{}" {
					t.Fatalf("marshalMetadata = %q, want {}", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("marshalMetadata = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckMetricsHighThresholds(t *testing.T) {
	t.Parallel()

	repo := &stubAlertsRepo{}
	mgr := &Manager{
		nowFn:    time.Now,
		hostname: func() (string, error) { return testHealthHost, nil },
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{
				ServicePath:  "/home/dev/.config/systemd/user/sentinel.service",
				ActiveState:  "active",
				EnabledState: "enabled",
			}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{TimerActiveState: "active", TimerEnabledState: "enabled"}, nil
		},
	}

	// Set thresholds very low so real metrics will exceed them.
	hc := &HealthChecker{
		manager:    mgr,
		alerts:     repo,
		thresholds: AlertThresholds{CPUPercent: 0.001, MemPercent: 0.001, DiskPercent: 0.001},
	}

	hc.checkMetrics(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	// At minimum memory and disk should be above 0.001%.
	if len(repo.upserted) == 0 {
		t.Fatal("expected at least one metric alert with very low thresholds")
	}
}
