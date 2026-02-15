package ops

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/service"
)

const testHostname = "host-a"

func TestListServices(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		startedAt: fixedNow.Add(-10 * time.Minute),
		nowFn:     func() time.Time { return fixedNow },
		hostname:  func() (string, error) { return testHostname, nil },
		uidFn:     func() int { return 1000 },
		goos:      "linux",
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(scope string) (service.UserAutoUpdateServiceStatus, error) {
			if scope != scopeUser {
				t.Fatalf("scope = %q, want %q", scope, scopeUser)
			}
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: true,
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "active",
				LastRunState:      "inactive",
			}, nil
		},
	}

	services, err := m.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("len(services) = %d, want 2", len(services))
	}
	if services[0].Name != ServiceNameSentinel || services[0].Scope != scopeUser {
		t.Fatalf("unexpected sentinel status: %+v", services[0])
	}
	if services[1].Name != ServiceNameUpdater || services[1].Unit != updaterSystemdUnit {
		t.Fatalf("unexpected updater status: %+v", services[1])
	}
	if services[1].UpdatedAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("updatedAt = %q, want %q", services[1].UpdatedAt, fixedNow.Format(time.RFC3339))
	}
}

func TestOverview(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		startedAt: fixedNow.Add(-2 * time.Hour),
		nowFn:     func() time.Time { return fixedNow },
		hostname:  func() (string, error) { return testHostname, nil },
		uidFn:     func() int { return 1000 },
		goos:      "linux",
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (service.UserAutoUpdateServiceStatus, error) {
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: true,
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "failed",
				LastRunState:      "inactive",
			}, nil
		},
	}

	overview, err := m.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if overview.Services.Total != 2 || overview.Services.Active != 1 || overview.Services.Failed != 1 {
		t.Fatalf("unexpected services summary: %+v", overview.Services)
	}
	if overview.Sentinel.UptimeSec != 2*60*60 {
		t.Fatalf("uptime = %d, want %d", overview.Sentinel.UptimeSec, 2*60*60)
	}
	if overview.Host.Hostname != testHostname {
		t.Fatalf("hostname = %q, want %s", overview.Host.Hostname, testHostname)
	}
}

func TestActSystemdUser(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := &Manager{
		nowFn:    time.Now,
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		hostname: func() (string, error) { return testHostname, nil },
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (service.UserAutoUpdateServiceStatus, error) {
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: true,
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "active",
			}, nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			return "", nil
		},
	}

	status, err := m.Act(context.Background(), "sentinel", "restart")
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if status.Name != ServiceNameSentinel {
		t.Fatalf("status.Name = %q, want %q", status.Name, ServiceNameSentinel)
	}
	want := []string{"systemctl", "--user", "restart", sentinelSystemdUnit}
	if len(calls) == 0 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("first call = %v, want %v", calls, want)
	}
}

func TestActSystemdUpdater(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := &Manager{
		nowFn:    time.Now,
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		hostname: func() (string, error) { return testHostname, nil },
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (service.UserAutoUpdateServiceStatus, error) {
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: true,
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "active",
			}, nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			return "", nil
		},
	}

	_, err := m.Act(context.Background(), ServiceNameUpdater, ActionStop)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	want := []string{"systemctl", "--user", "stop", updaterSystemdUnit}
	if len(calls) == 0 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("first call = %v, want %v", calls, want)
	}
}

func TestActLaunchdStartBootstrapsWhenMissing(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := &Manager{
		nowFn:    time.Now,
		uidFn:    func() int { return 1000 },
		goos:     "darwin",
		hostname: func() (string, error) { return testHostname, nil },
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist",
				UnitFileExists: true,
				EnabledState:   "loaded",
				ActiveState:    "inactive",
			}, nil
		},
		autoUpdateStatusFn: func(string) (service.UserAutoUpdateServiceStatus, error) {
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: true,
				TimerUnitExists:   true,
				TimerEnabledState: "loaded",
				TimerActiveState:  "inactive",
			}, nil
		},
		userServicePathFn: func() (string, error) {
			return "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist", nil
		},
		autoServicePathFn: func(string) (string, error) {
			return "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.updater.plist", nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			if len(args) > 0 && args[0] == "print" {
				return "", errors.New("launchctl print failed: Could not find service")
			}
			return "", nil
		},
	}

	_, err := m.Act(context.Background(), ServiceNameSentinel, ActionStart)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	expected := [][]string{
		{"launchctl", "print", "gui/1000/" + sentinelLaunchdLabel},
		{"launchctl", "bootstrap", "gui/1000", "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist"},
		{"launchctl", "kickstart", "-k", "gui/1000/" + sentinelLaunchdLabel},
	}
	if len(calls) < len(expected) {
		t.Fatalf("calls = %v, want at least %d", calls, len(expected))
	}
	for i := range expected {
		if !reflect.DeepEqual(calls[i], expected[i]) {
			t.Fatalf("call[%d] = %v, want %v", i, calls[i], expected[i])
		}
	}
}

func TestActValidatesInput(t *testing.T) {
	t.Parallel()

	m := NewManager(time.Now())
	if _, err := m.Act(context.Background(), "unknown", ActionStart); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("error = %v, want ErrServiceNotFound", err)
	}
	if _, err := m.Act(context.Background(), ServiceNameSentinel, "invalid"); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
}
