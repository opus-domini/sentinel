package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/service"
	"github.com/opus-domini/sentinel/internal/store"
)

type stubCustomServicesRepo struct {
	services []store.CustomService
	err      error
}

func (s *stubCustomServicesRepo) ListCustomServices(_ context.Context) ([]store.CustomService, error) {
	return s.services, s.err
}

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

func TestActSystemdUpdaterStartBootstrapsWhenMissing(t *testing.T) {
	t.Parallel()

	installed := false
	var installOpts service.InstallUserAutoUpdateOptions
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
			if installed {
				return service.UserAutoUpdateServiceStatus{
					ServiceUnitExists: true,
					TimerUnitExists:   true,
					TimerEnabledState: "enabled",
					TimerActiveState:  "active",
				}, nil
			}
			return service.UserAutoUpdateServiceStatus{
				ServiceUnitExists: false,
				TimerUnitExists:   false,
				TimerEnabledState: "not-found",
				TimerActiveState:  "inactive",
			}, nil
		},
		installAutoUpdate: func(opts service.InstallUserAutoUpdateOptions) error {
			installOpts = opts
			installed = true
			return nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			return "", nil
		},
	}

	status, err := m.Act(context.Background(), ServiceNameUpdater, ActionStart)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if !installed {
		t.Fatal("expected updater bootstrap install to run")
	}
	if installOpts.SystemdScope != scopeUser {
		t.Fatalf("scope = %q, want %q", installOpts.SystemdScope, scopeUser)
	}
	if !installOpts.Enable || !installOpts.Start {
		t.Fatalf("install opts should enable and start updater: %+v", installOpts)
	}
	if installOpts.ServiceUnit != ServiceNameSentinel {
		t.Fatalf("service unit = %q, want %q", installOpts.ServiceUnit, ServiceNameSentinel)
	}
	want := []string{"systemctl", "--user", "start", updaterSystemdUnit}
	if len(calls) == 0 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("first call = %v, want %v", calls, want)
	}
	if !status.Exists {
		t.Fatalf("status.Exists = false, want true")
	}
}

func TestActSystemdUpdaterRetriesAfterUnitNotFound(t *testing.T) {
	t.Parallel()

	attempt := 0
	installed := false
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
				TimerActiveState:  "inactive",
			}, nil
		},
		installAutoUpdate: func(opts service.InstallUserAutoUpdateOptions) error {
			_ = opts
			installed = true
			return nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			attempt++
			if attempt == 1 {
				return "", errors.New("systemctl --user start sentinel-updater.timer failed: Unit sentinel-updater.timer not found.")
			}
			return "", nil
		},
	}

	_, err := m.Act(context.Background(), ServiceNameUpdater, ActionStart)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if !installed {
		t.Fatal("expected updater bootstrap install to run after unit-not-found error")
	}
	if len(calls) != 2 {
		t.Fatalf("command attempts = %d, want 2", len(calls))
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

func TestInspectSystemdService(t *testing.T) {
	t.Parallel()

	m := &Manager{
		nowFn:    func() time.Time { return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC) },
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
		commandRunner: func(_ context.Context, _ string, args ...string) (string, error) {
			if len(args) < 2 || args[0] != "--user" || args[1] != "show" {
				t.Fatalf("unexpected command args: %v", args)
			}
			return strings.Join([]string{
				"Id=sentinel.service",
				"Description=Sentinel service",
				"LoadState=loaded",
				"UnitFileState=enabled",
				"ActiveState=active",
				"SubState=running",
				"FragmentPath=/home/dev/.config/systemd/user/sentinel.service",
				"ExecMainPID=1234",
			}, "\n"), nil
		},
	}

	details, err := m.Inspect(context.Background(), ServiceNameSentinel)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if details.Service.Name != ServiceNameSentinel {
		t.Fatalf("service = %q, want %q", details.Service.Name, ServiceNameSentinel)
	}
	if details.Properties["ActiveState"] != "active" {
		t.Fatalf("ActiveState = %q, want active", details.Properties["ActiveState"])
	}
	if details.Summary != "load=loaded active=active sub=running" {
		t.Fatalf("summary = %q, want systemd summary", details.Summary)
	}
	if details.CheckedAt != "2026-02-15T12:00:00Z" {
		t.Fatalf("checkedAt = %q, want fixed timestamp", details.CheckedAt)
	}
}

func TestInspectServiceNotFound(t *testing.T) {
	t.Parallel()

	m := NewManager(time.Now(), nil)
	if _, err := m.Inspect(context.Background(), "missing"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("error = %v, want ErrServiceNotFound", err)
	}
}

func TestActValidatesInput(t *testing.T) {
	t.Parallel()

	m := NewManager(time.Now(), nil)
	if _, err := m.Act(context.Background(), "unknown", ActionStart); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("error = %v, want ErrServiceNotFound", err)
	}
	if _, err := m.Act(context.Background(), ServiceNameSentinel, "invalid"); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
}

func TestListServicesMergesCustomServices(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	repo := &stubCustomServicesRepo{
		services: []store.CustomService{
			{Name: "nginx", DisplayName: "Nginx", Manager: "systemd", Unit: "nginx.service", Scope: "system"},
		},
	}
	m := &Manager{
		startedAt:      fixedNow.Add(-10 * time.Minute),
		nowFn:          func() time.Time { return fixedNow },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: repo,
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
		commandRunner: func(_ context.Context, _ string, _ ...string) (string, error) {
			return "ActiveState=active\nLoadState=loaded\nUnitFileState=enabled", nil
		},
	}

	services, err := m.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(services) != 3 {
		t.Fatalf("len(services) = %d, want 3 (sentinel + updater + nginx)", len(services))
	}
	if services[2].Name != "nginx" {
		t.Fatalf("services[2].Name = %q, want nginx", services[2].Name)
	}
	if services[2].Manager != "systemd" {
		t.Fatalf("services[2].Manager = %q, want systemd", services[2].Manager)
	}
}

func TestListServicesCustomServicesError(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	repo := &stubCustomServicesRepo{
		err: errors.New("db locked"),
	}
	m := &Manager{
		startedAt:      fixedNow.Add(-10 * time.Minute),
		nowFn:          func() time.Time { return fixedNow },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: repo,
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
	}

	services, err := m.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices should not fail when custom services error: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("len(services) = %d, want 2 (graceful degradation)", len(services))
	}
}
