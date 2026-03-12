package services

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/store"
)

type stubCustomServicesRepo struct {
	services []store.CustomService
	err      error
}

func (s *stubCustomServicesRepo) ListCustomServices(_ context.Context) ([]store.CustomService, error) {
	return s.services, s.err
}

// builtinServicesRepo returns a stub repo with sentinel and sentinel-updater
// registered as custom services, matching the systemd or launchd unit names.
func builtinServicesRepo(goos string) *stubCustomServicesRepo {
	manager := detectManager(goos)
	return &stubCustomServicesRepo{
		services: []store.CustomService{
			{
				Name:        ServiceNameSentinel,
				DisplayName: "Sentinel service",
				Manager:     manager,
				Unit:        unitForService(manager, ServiceNameSentinel),
				Scope:       scopeUser,
			},
			{
				Name:        ServiceNameUpdater,
				DisplayName: "Autoupdate timer",
				Manager:     manager,
				Unit:        unitForService(manager, ServiceNameUpdater),
				Scope:       scopeUser,
			},
		},
	}
}

const testHostname = "host-a"

// probeActiveResponse is a canned systemctl show response indicating a
// loaded, enabled and active unit. Used across multiple test files.
const probeActiveResponse = "UnitFileState=enabled\nActiveState=active\nLoadState=loaded\n"

func TestListServices(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		startedAt:      fixedNow.Add(-10 * time.Minute),
		nowFn:          func() time.Time { return fixedNow },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: builtinServicesRepo("linux"),
		commandRunner: func(_ context.Context, _ string, _ ...string) (string, error) {
			return probeActiveResponse, nil
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
		startedAt:      fixedNow.Add(-2 * time.Hour),
		nowFn:          func() time.Time { return fixedNow },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: builtinServicesRepo("linux"),
		commandRunner: func(_ context.Context, _ string, args ...string) (string, error) {
			if slices.Contains(args, updaterSystemdUnit) {
				return "UnitFileState=enabled\nActiveState=failed\nLoadState=loaded\n", nil
			}
			return probeActiveResponse, nil
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
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("linux"),
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
	found := false
	for _, c := range calls {
		if reflect.DeepEqual(c, want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %v among calls %v", want, calls)
	}
}

func TestActSystemdUpdater(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := &Manager{
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("linux"),
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
	found := false
	for _, c := range calls {
		if reflect.DeepEqual(c, want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %v among calls %v", want, calls)
	}
}

func TestActSystemdUpdaterStartBootstrapsWhenMissing(t *testing.T) {
	t.Parallel()

	installed := false
	var installOpts daemon.InstallUserAutoUpdateOptions
	var calls [][]string

	// The updater probe returns "not-found" until bootstrap installs it.
	updaterProbeResponse := "UnitFileState=not-found\nActiveState=inactive\nLoadState=not-found\n"
	m := &Manager{
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("linux"),
		installAutoUpdate: func(opts daemon.InstallUserAutoUpdateOptions) error {
			installOpts = opts
			installed = true
			// After bootstrap, probe will return loaded.
			updaterProbeResponse = probeActiveResponse
			return nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			// Handle probe calls (systemctl show).
			if name == cmdSystemctl && slices.Contains(args, "show") {
				if slices.Contains(args, updaterSystemdUnit) {
					return updaterProbeResponse, nil
				}
				return probeActiveResponse, nil
			}
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

	startAttempt := 0
	installed := false
	var startCalls [][]string
	m := &Manager{
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("linux"),
		installAutoUpdate: func(opts daemon.InstallUserAutoUpdateOptions) error {
			_ = opts
			installed = true
			return nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			// Handle probe calls (systemctl show).
			if name == cmdSystemctl && slices.Contains(args, "show") {
				return "UnitFileState=enabled\nActiveState=inactive\nLoadState=loaded\n", nil
			}
			row := append([]string{name}, args...)
			startCalls = append(startCalls, row)
			startAttempt++
			if startAttempt == 1 {
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
	if len(startCalls) != 2 {
		t.Fatalf("command attempts = %d, want 2", len(startCalls))
	}
}

func TestActLaunchdStartBootstrapsWhenMissing(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := &Manager{
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "darwin",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("darwin"),
		userServicePathFn: func() (string, error) {
			return "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist", nil
		},
		autoServicePathFn: func(string) (string, error) {
			return "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.updater.plist", nil
		},
		commandRunner: func(_ context.Context, name string, args ...string) (string, error) {
			row := append([]string{name}, args...)
			calls = append(calls, row)
			if len(args) > 0 && args[0] == argPrint {
				return "", errors.New("launchctl print failed: Could not find service")
			}
			return "", nil
		},
	}

	_, err := m.Act(context.Background(), ServiceNameSentinel, ActionStart)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	// First two calls are probes for sentinel and updater (both return "not found").
	// Then the action calls: print (check loaded) → bootstrap → kickstart.
	expected := [][]string{
		{"launchctl", "print", "gui/1000/" + sentinelLaunchdLabel},
		{"launchctl", "bootstrap", "gui/1000", "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist"},
		{"launchctl", "kickstart", "-k", "gui/1000/" + sentinelLaunchdLabel},
	}
	// Skip the initial probe calls (2 probes for list services before and after act).
	actionCalls := calls[2:] // first 2 are probes for ListServices
	// Filter to action calls: find the first "print" that's the loaded check
	found := false
	for i := range actionCalls {
		if len(actionCalls) >= i+len(expected) {
			match := true
			for j := range expected {
				if !reflect.DeepEqual(actionCalls[i+j], expected[j]) {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected action sequence %v within calls %v", expected, calls)
	}
}

func TestInspectSystemdService(t *testing.T) {
	t.Parallel()

	m := &Manager{
		nowFn:          func() time.Time { return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC) },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		hostname:       func() (string, error) { return testHostname, nil },
		customServices: builtinServicesRepo("linux"),
		commandRunner: func(_ context.Context, _ string, args ...string) (string, error) {
			if !slices.Contains(args, "show") {
				return "", errors.New("unexpected non-show command")
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
	if details.Properties["ActiveState"] != stateActive {
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
	builtins := builtinServicesRepo("linux")
	repo := &stubCustomServicesRepo{
		services: append(builtins.services,
			store.CustomService{Name: "nginx", DisplayName: "Nginx", Manager: "systemd", Unit: "nginx.service", Scope: "system"},
		),
	}
	m := &Manager{
		startedAt:      fixedNow.Add(-10 * time.Minute),
		nowFn:          func() time.Time { return fixedNow },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: repo,
		commandRunner: func(_ context.Context, _ string, _ ...string) (string, error) {
			return probeActiveResponse, nil
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
	}

	_, err := m.ListServices(context.Background())
	if err == nil {
		t.Fatal("ListServices should fail when custom services repo returns an error")
	}
	if !strings.Contains(err.Error(), "db locked") {
		t.Fatalf("error = %v, want containing 'db locked'", err)
	}
}

const (
	cmdSystemctl = "systemctl"
	cmdLaunchctl = "launchctl"
	argPrint     = "print"
	argUser      = "--user"
	argBootout   = "bootout"
)

// newTestManager creates a Manager with sensible defaults for testing.
// Override fields after creation as needed.
func newTestManager(goos string, runner commandRunner) *Manager {
	if runner == nil {
		runner = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", nil
		}
	}
	return &Manager{
		startedAt:      time.Date(2026, 2, 15, 11, 0, 0, 0, time.UTC),
		nowFn:          func() time.Time { return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC) },
		hostname:       func() (string, error) { return testHostname, nil },
		uidFn:          func() int { return 1000 },
		goos:           goos,
		customServices: builtinServicesRepo(goos),
		commandRunner:  runner,
	}
}

func TestDetectScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		uid  int
		want string
	}{
		{
			name: "systemd system path",
			path: "/etc/systemd/system/sentinel.service",
			uid:  1000,
			want: scopeSystem,
		},
		{
			name: "systemd user path",
			path: "/home/dev/.config/systemd/user/sentinel.service",
			uid:  1000,
			want: scopeUser,
		},
		{
			name: "launchd system path",
			path: "/Library/LaunchDaemons/io.opusdomini.sentinel.plist",
			uid:  1000,
			want: scopeSystem,
		},
		{
			name: "launchd user path",
			path: "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist",
			uid:  1000,
			want: scopeUser,
		},
		{
			name: "unknown path root uid is system",
			path: "/some/unknown/path",
			uid:  0,
			want: scopeSystem,
		},
		{
			name: "unknown path non-root uid is user",
			path: "/some/unknown/path",
			uid:  1000,
			want: scopeUser,
		},
		{
			name: "unknown path nil uidFn is user",
			path: "/some/unknown/path",
			uid:  -1, // sentinel for nil uidFn
			want: scopeUser,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var uidFn func() int
			if tc.uid >= 0 {
				uid := tc.uid
				uidFn = func() int { return uid }
			}
			got := detectScope(tc.path, uidFn)
			if got != tc.want {
				t.Fatalf("detectScope(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestProbeCustomServiceLaunchd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runner     commandRunner
		wantExists bool
		wantActive string
	}{
		{
			name: "launchd service found",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "loaded service info", nil
			},
			wantExists: true,
			wantActive: stateRunning,
		},
		{
			name: "launchd service not found",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", errors.New("Could not find service")
			},
			wantExists: false,
			wantActive: "inactive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				uidFn:         func() int { return 1000 },
				commandRunner: tc.runner,
			}
			svc := &ServiceStatus{
				Manager: managerLaunchd,
				Unit:    "com.example.app",
				Scope:   scopeUser,
			}
			m.probeCustomService(context.Background(), svc)
			if svc.Exists != tc.wantExists {
				t.Fatalf("Exists = %v, want %v", svc.Exists, tc.wantExists)
			}
			if svc.ActiveState != tc.wantActive {
				t.Fatalf("ActiveState = %q, want %q", svc.ActiveState, tc.wantActive)
			}
		})
	}
}

func TestProbeCustomServiceUnknownManager(t *testing.T) {
	t.Parallel()

	m := &Manager{}
	svc := &ServiceStatus{Manager: "openrc", Unit: "test"}
	m.probeCustomService(context.Background(), svc)
	if svc.Exists {
		t.Fatal("Exists should be false for unknown manager")
	}
	if svc.ActiveState != stateUnknown {
		t.Fatalf("ActiveState = %q, want %q", svc.ActiveState, stateUnknown)
	}
}

func TestInspectLaunchdService(t *testing.T) {
	t.Parallel()

	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdLaunchctl && len(args) > 0 && args[0] == argPrint {
			return "launchd service detail output", nil
		}
		return "", nil
	})

	details, err := m.Inspect(context.Background(), ServiceNameSentinel)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if details.Output != "launchd service detail output" {
		t.Fatalf("Output = %q, want launchd output", details.Output)
	}
}

func TestInspectLaunchdError(t *testing.T) {
	t.Parallel()

	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdLaunchctl && len(args) > 0 && args[0] == argPrint {
			return "", errors.New("launchctl failed")
		}
		return "", nil
	})

	_, err := m.Inspect(context.Background(), ServiceNameSentinel)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "launchd inspect failed") {
		t.Fatalf("error = %v, want 'launchd inspect failed'", err)
	}
}

func TestActLaunchdStop(t *testing.T) {
	t.Parallel()

	var calls [][]string
	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		row := append([]string{name}, args...)
		calls = append(calls, row)
		return "", nil
	})

	_, err := m.Act(context.Background(), ServiceNameSentinel, ActionStop)
	if err != nil {
		t.Fatalf("Act stop: %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("expected at least one command call")
	}
	// One of the calls should be bootout for stop.
	foundBootout := false
	for _, c := range calls {
		if c[0] == "launchctl" && len(c) > 1 && c[1] == "bootout" {
			foundBootout = true
			break
		}
	}
	if !foundBootout {
		t.Fatalf("expected launchctl bootout among calls: %v", calls)
	}
}

func TestActLaunchdStopMissingJobIgnored(t *testing.T) {
	t.Parallel()

	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdLaunchctl && len(args) > 0 && args[0] == argBootout {
			return "", errors.New("Could not find service")
		}
		return "", nil
	})

	_, err := m.Act(context.Background(), ServiceNameSentinel, ActionStop)
	if err != nil {
		t.Fatalf("Act stop with missing job should succeed: %v", err)
	}
}

func TestDiscoverServicesSystemd(t *testing.T) {
	t.Parallel()

	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdSystemctl && slices.Contains(args, "list-units") {
			return "nginx.service loaded active running Nginx web server\npostgres.service loaded active running PostgreSQL", nil
		}
		return "", nil
	})

	discovered, err := m.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}
	// Should not include built-in sentinel/updater units.
	for _, d := range discovered {
		if d.Unit == sentinelSystemdUnit || d.Unit == updaterSystemdUnit {
			t.Fatalf("discovered built-in unit %q, should be filtered", d.Unit)
		}
	}
	if len(discovered) == 0 {
		t.Fatal("expected at least one discovered service")
	}
	if discovered[0].Manager != managerSystemd {
		t.Fatalf("Manager = %q, want systemd", discovered[0].Manager)
	}
	if discovered[0].UnitType != unitTypeService {
		t.Fatalf("UnitType = %q, want service", discovered[0].UnitType)
	}
}

func TestDiscoverServicesLaunchd(t *testing.T) {
	t.Parallel()

	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdLaunchctl && len(args) > 0 && args[0] == "list" {
			return "PID\tStatus\tLabel\n123\t0\tcom.example.app\n-\t0\tcom.example.stopped", nil
		}
		return "", nil
	})

	discovered, err := m.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}
	for _, d := range discovered {
		if d.Unit == sentinelLaunchdLabel || d.Unit == updaterLaunchdLabel {
			t.Fatalf("discovered built-in unit %q, should be filtered", d.Unit)
		}
	}
	if len(discovered) == 0 {
		t.Fatal("expected at least one discovered service")
	}
	if discovered[0].Manager != managerLaunchd {
		t.Fatalf("Manager = %q, want launchd", discovered[0].Manager)
	}
	if discovered[0].UnitType != unitTypeJob {
		t.Fatalf("UnitType = %q, want job", discovered[0].UnitType)
	}
}

func TestDiscoverServicesDiscoverError(t *testing.T) {
	t.Parallel()

	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdSystemctl && slices.Contains(args, "list-units") {
			return "", errors.New("systemctl unavailable")
		}
		return "", nil
	})

	// Should not error — just returns empty (with a slog.Warn).
	discovered, err := m.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}
	if len(discovered) != 0 {
		t.Fatalf("expected 0 discovered services on error, got %d", len(discovered))
	}
}

func TestDiscoverServicesRootSkipsUserScope(t *testing.T) {
	t.Parallel()

	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdSystemctl && slices.Contains(args, "list-units") {
			if slices.Contains(args, argUser) {
				return "", errors.New("unexpected --user call when running as root")
			}
			return "nginx.service loaded active running Nginx web server", nil
		}
		return "", nil
	})
	m.uidFn = func() int { return 0 } // simulate root

	discovered, err := m.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}
	if len(discovered) == 0 {
		t.Fatal("expected at least one discovered service")
	}
	for _, d := range discovered {
		if d.Scope == scopeUser {
			t.Fatalf("discovered user-scope unit %q when running as root", d.Unit)
		}
	}
}

func TestBrowseServicesSystemd(t *testing.T) {
	t.Parallel()

	repo := &stubCustomServicesRepo{
		services: []store.CustomService{
			{Name: "nginx", DisplayName: "Nginx", Manager: "systemd", Unit: "nginx.service", Scope: "system"},
		},
	}

	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdSystemctl {
			if slices.Contains(args, "list-units") {
				return "nginx.service loaded active running Nginx\nredis.service loaded active running Redis", nil
			}
			if slices.Contains(args, "show") {
				return "ActiveState=active\nLoadState=loaded\nUnitFileState=enabled", nil
			}
		}
		return "", nil
	})
	m.customServices = repo

	result, err := m.BrowseServices(context.Background())
	if err != nil {
		t.Fatalf("BrowseServices: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one browsed service")
	}

	// nginx should be marked as tracked.
	foundTracked := false
	foundUntracked := false
	for _, bs := range result {
		if bs.Unit == "nginx.service" && bs.Scope == "system" && bs.Tracked {
			foundTracked = true
			if bs.TrackedName != "nginx" {
				t.Fatalf("TrackedName = %q, want nginx", bs.TrackedName)
			}
		}
		if bs.Unit == "redis.service" && !bs.Tracked {
			foundUntracked = true
		}
	}
	if !foundTracked {
		t.Fatal("expected nginx.service to be marked as tracked")
	}
	if !foundUntracked {
		t.Fatal("expected redis.service to be untracked")
	}
	for _, bs := range result {
		if bs.UnitType != unitTypeService {
			t.Fatalf("UnitType(%s) = %q, want service", bs.Unit, bs.UnitType)
		}
	}
}

func TestBrowseServicesSystemdIncludesInactiveUnitFiles(t *testing.T) {
	t.Parallel()

	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name != cmdSystemctl {
			return "", nil
		}
		switch {
		case slices.Contains(args, "list-units"):
			return "nginx.service loaded active running Nginx", nil
		case slices.Contains(args, "list-unit-files"):
			return "nginx.service enabled\nsuspend.target static", nil
		case slices.Contains(args, "show"):
			return "ActiveState=active\nLoadState=loaded\nUnitFileState=enabled", nil
		default:
			return "", nil
		}
	})

	result, err := m.BrowseServices(context.Background())
	if err != nil {
		t.Fatalf("BrowseServices: %v", err)
	}

	var found bool
	for _, bs := range result {
		if bs.Unit != "suspend.target" {
			continue
		}
		found = true
		if bs.UnitType != "target" {
			t.Fatalf("UnitType = %q, want target", bs.UnitType)
		}
		if bs.ActiveState != stateInactive {
			t.Fatalf("ActiveState = %q, want inactive", bs.ActiveState)
		}
		if bs.EnabledState != "static" {
			t.Fatalf("EnabledState = %q, want static", bs.EnabledState)
		}
	}
	if !found {
		t.Fatal("expected suspend.target from list-unit-files to be included")
	}
}

func TestBrowseServicesLaunchd(t *testing.T) {
	t.Parallel()

	m := newTestManager("darwin", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdLaunchctl && len(args) > 0 && args[0] == "list" {
			return "PID\tStatus\tLabel\n123\t0\tcom.example.app", nil
		}
		return "", nil
	})

	result, err := m.BrowseServices(context.Background())
	if err != nil {
		t.Fatalf("BrowseServices: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one browsed service")
	}
	if result[0].UnitType != unitTypeJob {
		t.Fatalf("UnitType = %q, want job", result[0].UnitType)
	}
}

func TestBrowseServicesInjectsTracked(t *testing.T) {
	t.Parallel()

	// When discover returns nothing, tracked services should still appear.
	m := newTestManager("linux", func(_ context.Context, name string, args ...string) (string, error) {
		if name == cmdSystemctl && slices.Contains(args, "list-units") {
			return "", nil // No discovered services.
		}
		return "", nil
	})

	result, err := m.BrowseServices(context.Background())
	if err != nil {
		t.Fatalf("BrowseServices: %v", err)
	}
	// Should still contain the built-in sentinel + updater services.
	if len(result) < 2 {
		t.Fatalf("expected at least 2 tracked services injected, got %d", len(result))
	}
	for _, bs := range result {
		if !bs.Tracked {
			t.Fatalf("expected all injected services to be tracked, %q is not", bs.Unit)
		}
	}
}

func TestActByUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		unit    string
		scope   string
		manager string
		action  string
		runner  commandRunner
		wantErr string
	}{
		{
			name:    "invalid action",
			unit:    "test.service",
			scope:   "user",
			manager: "systemd",
			action:  "nuke",
			wantErr: "ops invalid action",
		},
		{
			name:    "unsupported manager",
			unit:    "test",
			scope:   "user",
			manager: "openrc",
			action:  "start",
			wantErr: "unsupported service manager",
		},
		{
			name:    "systemd start success",
			unit:    "nginx.service",
			scope:   "user",
			manager: "systemd",
			action:  "start",
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name != cmdSystemctl {
					return "", errors.New("expected systemctl")
				}
				if args[0] != argUser {
					return "", errors.New("expected --user for user scope")
				}
				return "", nil
			},
		},
		{
			name:    "systemd system scope no --user flag",
			unit:    "nginx.service",
			scope:   "system",
			manager: "systemd",
			action:  "restart",
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if args[0] == argUser {
					return "", errors.New("unexpected --user flag for system scope")
				}
				return "", nil
			},
		},
		{
			name:    "systemd error propagates",
			unit:    "nginx.service",
			scope:   "user",
			manager: "systemd",
			action:  "stop",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", errors.New("access denied")
			},
			wantErr: "systemd action failed",
		},
		{
			name:    "launchd stop success",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			action:  "stop",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", nil
			},
		},
		{
			name:    "launchd stop missing job ignored",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			action:  "stop",
			runner: func(_ context.Context, _ string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == argBootout {
					return "", errors.New("Could not find service")
				}
				return "", nil
			},
		},
		{
			name:    "launchd start not loaded returns error",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			action:  "start",
			runner: func(_ context.Context, _ string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == argPrint {
					return "", errors.New("Could not find service")
				}
				return "", nil
			},
			wantErr: "launchd service com.example.app is not loaded",
		},
		{
			name:    "launchd start loaded success",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			action:  "start",
			runner: func(_ context.Context, _ string, args ...string) (string, error) {
				return "ok", nil
			},
		},
		{
			name:    "launchd invalid action",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			action:  "noop",
			wantErr: "ops invalid action",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				uidFn:         func() int { return 1000 },
				commandRunner: tc.runner,
			}
			err := m.ActByUnit(context.Background(), tc.unit, tc.scope, tc.manager, tc.action)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestInspectByUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		unit    string
		scope   string
		manager string
		runner  commandRunner
		wantErr string
	}{
		{
			name:    "systemd inspect",
			unit:    "nginx.service",
			scope:   "user",
			manager: "systemd",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "LoadState=loaded\nActiveState=active\nSubState=running", nil
			},
		},
		{
			name:    "systemd inspect error",
			unit:    "nginx.service",
			scope:   "user",
			manager: "systemd",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", errors.New("systemctl failed")
			},
			wantErr: "systemd inspect failed",
		},
		{
			name:    "launchd inspect",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "launchd detail", nil
			},
		},
		{
			name:    "launchd inspect error",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			runner: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", errors.New("launchctl failed")
			},
			wantErr: "launchd inspect failed",
		},
		{
			name:    "unsupported manager",
			unit:    "test",
			scope:   "user",
			manager: "openrc",
			wantErr: "unsupported service manager",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				nowFn:         func() time.Time { return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC) },
				uidFn:         func() int { return 1000 },
				commandRunner: tc.runner,
			}
			result, err := m.InspectByUnit(context.Background(), tc.unit, tc.scope, tc.manager)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Service.Unit != tc.unit {
				t.Fatalf("Unit = %q, want %q", result.Service.Unit, tc.unit)
			}
			if result.CheckedAt == "" {
				t.Fatal("CheckedAt is empty")
			}
		})
	}
}

func TestDiscoverSystemdUnitsParsing(t *testing.T) {
	t.Parallel()

	m := &Manager{
		commandRunner: func(_ context.Context, _ string, args ...string) (string, error) {
			switch {
			case slices.Contains(args, "list-units"):
				// Simulate systemctl list-units output: UNIT LOAD ACTIVE SUB DESCRIPTION...
				return "nginx.service loaded active running Nginx web server\n" +
					"redis.service loaded active running Redis\n" +
					"short\n" +
					"", nil
			case slices.Contains(args, "list-unit-files"):
				return "nginx.service enabled\nredis.service disabled\n", nil
			default:
				return "", nil
			}
		},
	}

	units, err := m.discoverSystemdUnits(context.Background(), scopeUser)
	if err != nil {
		t.Fatalf("discoverSystemdUnits: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("len(units) = %d, want 2 (short line filtered)", len(units))
	}
	if units[0].Unit != "nginx.service" {
		t.Fatalf("units[0].Unit = %q, want nginx.service", units[0].Unit)
	}
	if units[0].Description != "Nginx web server" {
		t.Fatalf("units[0].Description = %q, want 'Nginx web server'", units[0].Description)
	}
	if units[0].UnitType != unitTypeService {
		t.Fatalf("units[0].UnitType = %q, want service", units[0].UnitType)
	}
	if units[1].UnitType != unitTypeService {
		t.Fatalf("units[1].UnitType = %q, want service", units[1].UnitType)
	}
	if units[0].EnabledState != "enabled" {
		t.Fatalf("units[0].EnabledState = %q, want enabled", units[0].EnabledState)
	}
	if units[1].EnabledState != "disabled" {
		t.Fatalf("units[1].EnabledState = %q, want disabled", units[1].EnabledState)
	}
}

func TestDiscoverSystemdUnitsIncludesUnitFilesNotLoaded(t *testing.T) {
	t.Parallel()

	m := &Manager{
		commandRunner: func(_ context.Context, _ string, args ...string) (string, error) {
			switch {
			case slices.Contains(args, "list-units"):
				return "nginx.service loaded active running Nginx web server\n", nil
			case slices.Contains(args, "list-unit-files"):
				return "nginx.service enabled\nsuspend.target static\n", nil
			default:
				return "", nil
			}
		},
	}

	units, err := m.discoverSystemdUnits(context.Background(), scopeUser)
	if err != nil {
		t.Fatalf("discoverSystemdUnits: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("len(units) = %d, want 2", len(units))
	}
	if units[1].Unit != "suspend.target" {
		t.Fatalf("units[1].Unit = %q, want suspend.target", units[1].Unit)
	}
	if units[1].UnitType != "target" {
		t.Fatalf("units[1].UnitType = %q, want target", units[1].UnitType)
	}
	if units[1].ActiveState != stateInactive {
		t.Fatalf("units[1].ActiveState = %q, want inactive", units[1].ActiveState)
	}
	if units[1].EnabledState != "static" {
		t.Fatalf("units[1].EnabledState = %q, want static", units[1].EnabledState)
	}
}

func TestDiscoverLaunchdUnitsParsing(t *testing.T) {
	t.Parallel()

	m := &Manager{
		commandRunner: func(_ context.Context, _ string, _ ...string) (string, error) {
			return "PID\tStatus\tLabel\n" +
				"123\t0\tcom.apple.Finder\n" +
				"-\t0\tcom.example.stopped\n" +
				"\n" +
				"ab\n", nil // short line skipped
		},
	}

	units, err := m.discoverLaunchdUnits(context.Background())
	if err != nil {
		t.Fatalf("discoverLaunchdUnits: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("len(units) = %d, want 2", len(units))
	}
	if units[0].ActiveState != stateActive {
		t.Fatalf("units[0].ActiveState = %q, want active (PID present)", units[0].ActiveState)
	}
	if units[1].ActiveState != "inactive" {
		t.Fatalf("units[1].ActiveState = %q, want inactive (PID is '-')", units[1].ActiveState)
	}
	if units[0].UnitType != unitTypeJob || units[1].UnitType != unitTypeJob {
		t.Fatalf("launchd units should be typed as job: %+v", units)
	}
}

func TestBrowseUnitType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		manager string
		unit    string
		want    string
	}{
		{name: "systemd service suffix", manager: managerSystemd, unit: "nginx.service", want: unitTypeService},
		{name: "systemd target suffix", manager: managerSystemd, unit: "multi-user.target", want: "target"},
		{name: "systemd no suffix defaults to service", manager: managerSystemd, unit: "sentinel", want: unitTypeService},
		{name: "launchd labels are jobs", manager: managerLaunchd, unit: "io.opusdomini.sentinel", want: unitTypeJob},
		{name: "unknown manager falls back to unit", manager: "openrc", unit: "nginx", want: "unit"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := browseUnitType(tc.manager, tc.unit); got != tc.want {
				t.Fatalf("browseUnitType(%q, %q) = %q, want %q", tc.manager, tc.unit, got, tc.want)
			}
		})
	}
}

func TestIsLaunchdAlreadyLoadedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "already loaded",
			err:  errors.New("launchctl bootstrap failed: Service already loaded"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isLaunchdAlreadyLoadedError(tc.err)
			if got != tc.want {
				t.Fatalf("isLaunchdAlreadyLoadedError = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSystemdShow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "standard properties",
			input: "ActiveState=active\nSubState=running\nLoadState=loaded",
			want:  map[string]string{"ActiveState": "active", "SubState": "running", "LoadState": "loaded"},
		},
		{
			name:  "empty value",
			input: "Description=\nActiveState=active",
			want:  map[string]string{"Description": "", "ActiveState": "active"},
		},
		{
			name:  "no equals sign skipped",
			input: "some random line\nActiveState=active",
			want:  map[string]string{"ActiveState": "active"},
		},
		{
			name:  "equals at start skipped",
			input: "=value\nActiveState=active",
			want:  map[string]string{"ActiveState": "active"},
		},
		{
			name:  "empty input",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "blank lines skipped",
			input: "\n\nActiveState=active\n\n",
			want:  map[string]string{"ActiveState": "active"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseSystemdShow(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d: got %v", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestBuildInspectSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		props map[string]string
		want  string
	}{
		{
			name:  "nil props",
			props: nil,
			want:  "",
		},
		{
			name:  "empty props",
			props: map[string]string{},
			want:  "",
		},
		{
			name:  "all fields",
			props: map[string]string{"LoadState": "loaded", "ActiveState": "active", "SubState": "running"},
			want:  "load=loaded active=active sub=running",
		},
		{
			name:  "partial fields",
			props: map[string]string{"ActiveState": "failed"},
			want:  "active=failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildInspectSummary(tc.props)
			if got != tc.want {
				t.Fatalf("buildInspectSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLaunchdDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope string
		uid   int
		want  string
	}{
		{
			name:  "system scope",
			scope: "system",
			uid:   1000,
			want:  scopeSystem,
		},
		{
			name:  "user scope",
			scope: "user",
			uid:   501,
			want:  "gui/501",
		},
		{
			name:  "user scope nil uidFn",
			scope: "user",
			uid:   -1,
			want:  "gui/0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var uidFn func() int
			if tc.uid >= 0 {
				uid := tc.uid
				uidFn = func() int { return uid }
			}
			got := launchdDomain(tc.scope, uidFn)
			if got != tc.want {
				t.Fatalf("launchdDomain = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOverviewNegativeUptime(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	m := newTestManager("linux", nil)
	m.startedAt = fixedNow.Add(10 * time.Hour) // future = negative uptime
	m.nowFn = func() time.Time { return fixedNow }

	overview, err := m.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if overview.Sentinel.UptimeSec != 0 {
		t.Fatalf("UptimeSec = %d, want 0 for negative uptime", overview.Sentinel.UptimeSec)
	}
}

func TestNewManagerStartedAtDefault(t *testing.T) {
	t.Parallel()

	m := NewManager(time.Time{}, nil)
	if m.startedAt.IsZero() {
		t.Fatal("startedAt should not be zero when passed zero")
	}
}

func TestActUnsupportedManager(t *testing.T) {
	t.Parallel()

	// Act with a custom service that has an unsupported manager.
	repo := &stubCustomServicesRepo{
		services: []store.CustomService{
			{Name: "custom", DisplayName: "Custom", Manager: "openrc", Unit: "custom.service", Scope: "user"},
		},
	}
	m := newTestManager("linux", func(_ context.Context, _ string, _ ...string) (string, error) {
		return "ActiveState=unknown\nLoadState=not-found\nUnitFileState=unknown", nil
	})
	m.customServices = repo

	_, err := m.Act(context.Background(), "custom", ActionStart)
	if err == nil {
		t.Fatal("expected error for unsupported manager")
	}
	if !strings.Contains(err.Error(), "unsupported service manager") {
		t.Fatalf("error = %v, want 'unsupported service manager'", err)
	}
}

func TestInspectUnsupportedManager(t *testing.T) {
	t.Parallel()

	repo := &stubCustomServicesRepo{
		services: []store.CustomService{
			{Name: "custom", DisplayName: "Custom", Manager: "openrc", Unit: "custom.service", Scope: "user"},
		},
	}
	m := newTestManager("linux", func(_ context.Context, _ string, _ ...string) (string, error) {
		return "ActiveState=unknown\nLoadState=not-found\nUnitFileState=unknown", nil
	})
	m.customServices = repo

	_, err := m.Inspect(context.Background(), "custom")
	if err == nil {
		t.Fatal("expected error for unsupported manager")
	}
	if !strings.Contains(err.Error(), "unsupported service manager") {
		t.Fatalf("error = %v, want 'unsupported service manager'", err)
	}
}
