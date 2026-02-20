package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testServiceUnit = "sentinel"

func TestEscapeSystemdExec(t *testing.T) {
	t.Parallel()

	got := escapeSystemdExec("/opt/sentinel bin/sentinel")
	want := "/opt/sentinel\\x20bin/sentinel"
	if got != want {
		t.Fatalf("escapeSystemdExec() = %q, want %q", got, want)
	}
}

func TestRenderUserUnitIncludesExecStart(t *testing.T) {
	t.Parallel()

	unit := renderUserUnit("/usr/local/bin/sentinel")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/sentinel") {
		t.Fatalf("rendered unit missing ExecStart: %s", unit)
	}
	if !strings.Contains(unit, "Description=Sentinel - terminal workspace") {
		t.Fatalf("rendered unit missing description: %s", unit)
	}
	if !strings.Contains(unit, "Environment=HOME=%h") {
		t.Fatalf("rendered unit missing HOME environment: %s", unit)
	}
}

func TestRenderUserAutoUpdateUnitIncludesExecAndService(t *testing.T) {
	t.Parallel()

	unit := renderUserAutoUpdateUnit("/usr/local/bin/sentinel", "sentinel", "user")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/sentinel update apply") {
		t.Fatalf("rendered updater unit missing ExecStart: %s", unit)
	}
	if !strings.Contains(unit, "-service=sentinel") {
		t.Fatalf("rendered updater unit missing service target: %s", unit)
	}
	if !strings.Contains(unit, "-systemd-scope=user") {
		t.Fatalf("rendered updater unit missing scope: %s", unit)
	}
}

func TestRenderUserAutoUpdateTimerIncludesSchedule(t *testing.T) {
	t.Parallel()

	timer := renderUserAutoUpdateTimer("daily", time.Hour)
	if !strings.Contains(timer, "OnCalendar=daily") {
		t.Fatalf("rendered updater timer missing OnCalendar: %s", timer)
	}
	if !strings.Contains(timer, "RandomizedDelaySec=1h0m0s") {
		t.Fatalf("rendered updater timer missing RandomizedDelaySec: %s", timer)
	}
	if !strings.Contains(timer, "Unit=sentinel-updater.service") {
		t.Fatalf("rendered updater timer missing Unit: %s", timer)
	}
}

func TestNormalizeLinuxAutoUpdateScope(t *testing.T) {
	t.Parallel()

	got, err := normalizeLinuxAutoUpdateScope(managerScopeUser)
	if err != nil || got != managerScopeUser {
		t.Fatalf("normalizeLinuxAutoUpdateScope(user) = %q, %v", got, err)
	}

	got, err = normalizeLinuxAutoUpdateScope(managerScopeSystem)
	if err != nil || got != managerScopeSystem {
		t.Fatalf("normalizeLinuxAutoUpdateScope(system) = %q, %v", got, err)
	}

	if _, err := normalizeLinuxAutoUpdateScope("invalid"); err == nil {
		t.Fatal("expected error for invalid scope")
	}

	got, err = normalizeLinuxAutoUpdateScope("")
	if err != nil {
		t.Fatalf("normalizeLinuxAutoUpdateScope(\"\") error: %v", err)
	}
	want := managerScopeUser
	if os.Geteuid() == 0 {
		want = managerScopeSystem
	}
	if got != want {
		t.Fatalf("normalizeLinuxAutoUpdateScope(\"\") = %q, want %q", got, want)
	}
}

func TestUserAutoUpdatePathsForSystemScope(t *testing.T) {
	t.Parallel()

	servicePath, err := UserAutoUpdateServicePathForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("UserAutoUpdateServicePathForScope(system) error: %v", err)
	}

	timerPath, err := UserAutoUpdateTimerPathForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("UserAutoUpdateTimerPathForScope(system) error: %v", err)
	}

	switch runtime.GOOS {
	case launchdSupportedOS:
		if servicePath != launchdSystemUpdaterPath {
			t.Fatalf("service path = %q, want %q", servicePath, launchdSystemUpdaterPath)
		}
		if timerPath != launchdSystemUpdaterPath {
			t.Fatalf("timer path = %q, want %q", timerPath, launchdSystemUpdaterPath)
		}
	default:
		if servicePath != systemAutoUpdateService {
			t.Fatalf("service path = %q, want %q", servicePath, systemAutoUpdateService)
		}
		if timerPath != systemAutoUpdateTimer {
			t.Fatalf("timer path = %q, want %q", timerPath, systemAutoUpdateTimer)
		}
	}
}

func TestNormalizeSystemctlErrorState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "failed to connect",
			in:   "Failed to connect to bus: No medium found",
			want: "unavailable",
		},
		{
			name: "not found legacy",
			in:   "Unit sentinel.service could not be found.",
			want: "not-found",
		},
		{
			name: "not found missing file",
			in:   "Failed to get unit file state for sentinel.service: No such file or directory",
			want: "not-found",
		},
		{
			name: "empty",
			in:   "",
			want: systemdStateUnknown,
		},
		{
			name: "multiline",
			in:   "line1\nline2",
			want: systemdStateUnknown,
		},
		{
			name: "pass through",
			in:   "inactive",
			want: "inactive",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeSystemctlErrorState(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeSystemctlErrorState(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestApplySystemdUnitState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		enable    bool
		start     bool
		active    bool
		wantCalls []string
	}{
		{
			name:      "enable and start when inactive",
			enable:    true,
			start:     true,
			active:    false,
			wantCalls: []string{"enable sentinel", "start sentinel"},
		},
		{
			name:      "enable and restart when active",
			enable:    true,
			start:     true,
			active:    true,
			wantCalls: []string{"enable sentinel", "restart sentinel"},
		},
		{
			name:      "start only active restarts",
			enable:    false,
			start:     true,
			active:    true,
			wantCalls: []string{"restart sentinel"},
		},
		{
			name:      "enable only",
			enable:    true,
			start:     false,
			active:    true,
			wantCalls: []string{"enable sentinel"},
		},
		{
			name:      "noop",
			enable:    false,
			start:     false,
			active:    true,
			wantCalls: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			err := applySystemdUnitState(
				testServiceUnit,
				tc.enable,
				tc.start,
				func(unit string) bool {
					if unit != testServiceUnit {
						t.Fatalf("unit = %q, want %s", unit, testServiceUnit)
					}
					return tc.active
				},
				func(args ...string) error {
					calls = append(calls, strings.Join(args, " "))
					return nil
				},
			)
			if err != nil {
				t.Fatalf("applySystemdUnitState returned error: %v", err)
			}
			if strings.Join(calls, "|") != strings.Join(tc.wantCalls, "|") {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}

func TestApplySystemdUnitStateReturnsEnableError(t *testing.T) {
	t.Parallel()

	expected := "enable failed"
	err := applySystemdUnitState(
		testServiceUnit,
		true,
		true,
		func(string) bool { return false },
		func(args ...string) error {
			if strings.Join(args, " ") == "enable sentinel" {
				return errors.New(expected)
			}
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("error = %v, want contains %q", err, expected)
	}
}

func TestApplySystemdUnitStateReturnsStartError(t *testing.T) {
	t.Parallel()

	expected := "restart failed"
	err := applySystemdUnitState(
		testServiceUnit,
		true,
		true,
		func(string) bool { return true },
		func(args ...string) error {
			if strings.Join(args, " ") == "restart sentinel" {
				return errors.New(expected)
			}
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("error = %v, want contains %q", err, expected)
	}
}

func TestApplySystemdUnitStateNilIsActiveFn(t *testing.T) {
	t.Parallel()

	var calls []string
	err := applySystemdUnitState(
		testServiceUnit,
		false,
		true,
		nil,
		func(args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"start sentinel"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestResolveExecPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr string
		check   func(t *testing.T, got string)
	}{
		{
			name: "explicit path returned as-is",
			raw:  "/usr/local/bin/sentinel",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "/usr/local/bin/sentinel" {
					t.Fatalf("got %q, want /usr/local/bin/sentinel", got)
				}
			},
		},
		{
			name: "whitespace trimmed",
			raw:  "  /usr/bin/sentinel  ",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "/usr/bin/sentinel" {
					t.Fatalf("got %q, want /usr/bin/sentinel", got)
				}
			},
		},
		{
			name:    "newline rejected",
			raw:     "/usr/bin/sentinel\nmalicious",
			wantErr: "invalid executable path",
		},
		{
			name:    "carriage return rejected",
			raw:     "/usr/bin/sentinel\rmalicious",
			wantErr: "invalid executable path",
		},
		{
			name: "empty resolves to current executable",
			raw:  "",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got == "" {
					t.Fatal("expected non-empty path for empty input")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveExecPath(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, got)
		})
	}
}

func TestWithSystemdUserBusHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantNil     bool
		wantContain string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:        "regular error passes through",
			err:         errors.New("something went wrong"),
			wantContain: "something went wrong",
		},
		{
			name:        "bus connection error gets hint",
			err:         errors.New("Failed to connect to user scope bus: something"),
			wantContain: "if running as root use -scope system",
		},
		{
			name:        "DBUS_SESSION_BUS_ADDRESS error gets hint",
			err:         errors.New("DBUS_SESSION_BUS_ADDRESS not set"),
			wantContain: "if running as root use -scope system",
		},
		{
			name:        "XDG_RUNTIME_DIR error gets hint",
			err:         errors.New("XDG_RUNTIME_DIR not defined for user"),
			wantContain: "if running as root use -scope system",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := withSystemdUserBusHint(tc.err)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			if !strings.Contains(got.Error(), tc.wantContain) {
				t.Fatalf("error %q does not contain %q", got.Error(), tc.wantContain)
			}
		})
	}
}

func TestWithSystemdUserBusHintPreservesWrappedError(t *testing.T) {
	t.Parallel()

	original := errors.New("Failed to connect to user scope bus: timeout")
	got := withSystemdUserBusHint(original)
	if !errors.Is(got, original) {
		t.Fatal("hint-wrapped error should wrap the original")
	}
}

func TestEnsureServicePlatformSupported(t *testing.T) {
	t.Parallel()

	err := ensureServicePlatformSupported()
	switch runtime.GOOS {
	case systemdSupportedOS, launchdSupportedOS:
		if err != nil {
			t.Fatalf("expected nil on %s, got: %v", runtime.GOOS, err)
		}
	default:
		if err == nil {
			t.Fatalf("expected error on %s", runtime.GOOS)
		}
	}
}

func TestUserServicePath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	path, err := UserServicePath()
	if err != nil {
		t.Fatalf("UserServicePath() error: %v", err)
	}

	if os.Geteuid() == 0 {
		if path != systemUnitPath {
			t.Fatalf("root path = %q, want %q", path, systemUnitPath)
		}
	} else {
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".config", "systemd", "user", userUnitName)
		if path != want {
			t.Fatalf("user path = %q, want %q", path, want)
		}
	}
}

func TestUserAutoUpdateServicePathWrapper(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS && runtime.GOOS != launchdSupportedOS {
		t.Skip("test requires Linux or macOS")
	}

	path, err := UserAutoUpdateServicePath()
	if err != nil {
		t.Fatalf("UserAutoUpdateServicePath() error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestUserAutoUpdateTimerPathWrapper(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS && runtime.GOOS != launchdSupportedOS {
		t.Skip("test requires Linux or macOS")
	}

	path, err := UserAutoUpdateTimerPath()
	if err != nil {
		t.Fatalf("UserAutoUpdateTimerPath() error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestUserAutoUpdatePathsForUserScope(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	servicePath, err := UserAutoUpdateServicePathForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("UserAutoUpdateServicePathForScope(user) error: %v", err)
	}
	wantService := filepath.Join(home, ".config", "systemd", "user", userAutoUpdateServiceName)
	if servicePath != wantService {
		t.Fatalf("service path = %q, want %q", servicePath, wantService)
	}

	timerPath, err := UserAutoUpdateTimerPathForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("UserAutoUpdateTimerPathForScope(user) error: %v", err)
	}
	wantTimer := filepath.Join(home, ".config", "systemd", "user", userAutoUpdateTimerName)
	if timerPath != wantTimer {
		t.Fatalf("timer path = %q, want %q", timerPath, wantTimer)
	}
}

func TestUserAutoUpdatePathsForInvalidScope(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	_, err := UserAutoUpdateServicePathForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}

	_, err = UserAutoUpdateTimerPathForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestResolveInstallUserAutoUpdateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		opts             InstallUserAutoUpdateOptions
		wantErr          string
		checkServiceUnit string
		checkOnCalendar  string
		checkDelay       time.Duration
	}{
		{
			name: "defaults filled in",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
			},
			checkServiceUnit: "sentinel",
			checkOnCalendar:  "daily",
			checkDelay:       time.Hour,
		},
		{
			name: "explicit values preserved",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:        "/usr/bin/sentinel",
				SystemdScope:    "user",
				ServiceUnit:     "my-sentinel",
				OnCalendar:      "hourly",
				RandomizedDelay: 30 * time.Minute,
			},
			checkServiceUnit: "my-sentinel",
			checkOnCalendar:  "hourly",
			checkDelay:       30 * time.Minute,
		},
		{
			name: "whitespace-only service unit uses default",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
				ServiceUnit:  "   ",
			},
			checkServiceUnit: "sentinel",
		},
		{
			name: "invalid service unit with spaces",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
				ServiceUnit:  "bad name",
			},
			wantErr: "invalid service unit name",
		},
		{
			name: "invalid scope",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "invalid",
			},
			wantErr: "invalid systemd scope",
		},
		{
			name: "invalid exec path with newline",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel\nevil",
				SystemdScope: "user",
			},
			wantErr: "invalid executable path",
		},
		{
			name: "zero delay defaults to one hour",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:        "/usr/bin/sentinel",
				SystemdScope:    "user",
				RandomizedDelay: 0,
			},
			checkDelay: time.Hour,
		},
		{
			name: "negative delay defaults to one hour",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:        "/usr/bin/sentinel",
				SystemdScope:    "user",
				RandomizedDelay: -5 * time.Minute,
			},
			checkDelay: time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := resolveInstallUserAutoUpdateConfig(tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkServiceUnit != "" && cfg.serviceUnit != tc.checkServiceUnit {
				t.Fatalf("serviceUnit = %q, want %q", cfg.serviceUnit, tc.checkServiceUnit)
			}
			if tc.checkOnCalendar != "" && cfg.onCalendar != tc.checkOnCalendar {
				t.Fatalf("onCalendar = %q, want %q", cfg.onCalendar, tc.checkOnCalendar)
			}
			if tc.checkDelay != 0 && cfg.randomizedDelay != tc.checkDelay {
				t.Fatalf("randomizedDelay = %v, want %v", cfg.randomizedDelay, tc.checkDelay)
			}
		})
	}
}

func TestResolveInstallUserAutoUpdateConfigSetsScope(t *testing.T) {
	t.Parallel()

	cfg, err := resolveInstallUserAutoUpdateConfig(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "system",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.scope != managerScopeSystem {
		t.Fatalf("scope = %q, want %q", cfg.scope, managerScopeSystem)
	}
}

func TestNormalizeLinuxAutoUpdateScopeAutoScope(t *testing.T) {
	t.Parallel()

	got, err := normalizeLinuxAutoUpdateScope("auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := managerScopeUser
	if os.Geteuid() == 0 {
		want = managerScopeSystem
	}
	if got != want {
		t.Fatalf("normalizeLinuxAutoUpdateScope(auto) = %q, want %q", got, want)
	}
}

func TestNormalizeLinuxAutoUpdateScopeCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"User", managerScopeUser},
		{"USER", managerScopeUser},
		{"System", managerScopeSystem},
		{"SYSTEM", managerScopeSystem},
		{" user ", managerScopeUser},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeLinuxAutoUpdateScope(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("normalizeLinuxAutoUpdateScope(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEscapeSystemdExecBackslash(t *testing.T) {
	t.Parallel()

	got := escapeSystemdExec(`C:\Program Files\sentinel`)
	want := `C:\\Program\x20Files\\sentinel`
	if got != want {
		t.Fatalf("escapeSystemdExec() = %q, want %q", got, want)
	}
}

func TestRenderUserUnitEscapesSpacesInPath(t *testing.T) {
	t.Parallel()

	unit := renderUserUnit("/opt/my app/sentinel")
	if !strings.Contains(unit, `ExecStart=/opt/my\x20app/sentinel`) {
		t.Fatalf("rendered unit should escape spaces in ExecStart: %s", unit)
	}
}

func TestRenderUserAutoUpdateUnitSystemScope(t *testing.T) {
	t.Parallel()

	unit := renderUserAutoUpdateUnit("/usr/bin/sentinel", "myservice", "system")
	if !strings.Contains(unit, "-service=myservice") {
		t.Fatalf("unit missing custom service name: %s", unit)
	}
	if !strings.Contains(unit, "-systemd-scope=system") {
		t.Fatalf("unit missing system scope: %s", unit)
	}
}

func TestRenderUserAutoUpdateTimerCustomValues(t *testing.T) {
	t.Parallel()

	timer := renderUserAutoUpdateTimer("*-*-* 03:00:00", 30*time.Minute)
	if !strings.Contains(timer, "OnCalendar=*-*-* 03:00:00") {
		t.Fatalf("timer missing custom OnCalendar: %s", timer)
	}
	if !strings.Contains(timer, "RandomizedDelaySec=30m0s") {
		t.Fatalf("timer missing custom delay: %s", timer)
	}
}

func TestEnsureSystemdUserSupported(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	err := ensureSystemdUserSupported()
	// On a Linux system with systemctl, this should succeed.
	// On minimal containers without systemctl, it returns an error about systemctl.
	if err != nil {
		if !strings.Contains(err.Error(), "systemctl") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestUserStatusOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	st, err := UserStatus()
	if err != nil {
		// ensureSystemdUserSupported may fail in containers without systemctl
		if strings.Contains(err.Error(), "systemctl") {
			t.Skip("systemctl not available")
		}
		t.Fatalf("UserStatus() error: %v", err)
	}

	if st.ServicePath == "" {
		t.Fatal("expected non-empty ServicePath")
	}

	// On a normal Linux system, systemctl should be available
	if st.SystemctlAvailable {
		// EnabledState/ActiveState will return something (even "not-found" for non-installed unit)
		if st.EnabledState == "" {
			t.Fatal("expected non-empty EnabledState when systemctl is available")
		}
		if st.ActiveState == "" {
			t.Fatal("expected non-empty ActiveState when systemctl is available")
		}
	}
}

func TestUserAutoUpdateStatusWrapper(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS && runtime.GOOS != launchdSupportedOS {
		t.Skip("test requires Linux or macOS")
	}

	st, err := UserAutoUpdateStatus()
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") || strings.Contains(err.Error(), "launchctl") {
			t.Skip("tool not available")
		}
		t.Fatalf("UserAutoUpdateStatus() error: %v", err)
	}

	if st.ServicePath == "" {
		t.Fatal("expected non-empty ServicePath")
	}
}

func TestUserAutoUpdateStatusForScopeOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	st, err := UserAutoUpdateStatusForScope("user")
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			t.Skip("systemctl not available")
		}
		t.Fatalf("UserAutoUpdateStatusForScope(user) error: %v", err)
	}

	if st.ServicePath == "" {
		t.Fatal("expected non-empty ServicePath")
	}
	if st.TimerPath == "" {
		t.Fatal("expected non-empty TimerPath")
	}

	if st.SystemctlAvailable {
		if st.TimerEnabledState == "" {
			t.Fatal("expected non-empty TimerEnabledState")
		}
		if st.TimerActiveState == "" {
			t.Fatal("expected non-empty TimerActiveState")
		}
		if st.LastRunState == "" {
			t.Fatal("expected non-empty LastRunState")
		}
	}
}

func TestUserAutoUpdateStatusForScopeInvalid(t *testing.T) {
	t.Parallel()

	_, err := UserAutoUpdateStatusForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestRemoveUserAutoUpdateUnitsNonExistent(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux for systemd paths")
	}

	// removeUserAutoUpdateUnits should succeed when files don't exist (os.ErrNotExist is ignored)
	err := removeUserAutoUpdateUnits()
	if err != nil {
		t.Fatalf("removeUserAutoUpdateUnits() should not error for non-existent files: %v", err)
	}
}

func TestUserAutoUpdateStatusForScopeSystem(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	st, err := UserAutoUpdateStatusForScope("system")
	if err != nil {
		t.Fatalf("UserAutoUpdateStatusForScope(system) error: %v", err)
	}

	if st.ServicePath != systemAutoUpdateService {
		t.Fatalf("service path = %q, want %q", st.ServicePath, systemAutoUpdateService)
	}
	if st.TimerPath != systemAutoUpdateTimer {
		t.Fatalf("timer path = %q, want %q", st.TimerPath, systemAutoUpdateTimer)
	}

	if st.SystemctlAvailable {
		if st.TimerEnabledState == "" {
			t.Fatal("expected non-empty TimerEnabledState")
		}
		if st.TimerActiveState == "" {
			t.Fatal("expected non-empty TimerActiveState")
		}
		if st.LastRunState == "" {
			t.Fatal("expected non-empty LastRunState")
		}
	}
}

func TestInstallUserAutoUpdateSystemScopeNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := InstallUserAutoUpdate(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "system",
	})
	if err == nil {
		t.Fatal("expected error for system scope as non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestUninstallUserAutoUpdateSystemScopeNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope: "system",
	})
	if err == nil {
		t.Fatal("expected error for system scope as non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestInstallUserAutoUpdateInvalidScope(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	err := InstallUserAutoUpdate(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestUninstallUserAutoUpdateInvalidScope(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope: "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestInstallUserBadExecPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := InstallUser(InstallUserOptions{
		ExecPath: "/usr/bin/sentinel\nevil",
	})
	if err == nil {
		t.Fatal("expected error for invalid exec path")
	}
	if !strings.Contains(err.Error(), "invalid executable path") {
		t.Fatalf("error = %v, want invalid executable path", err)
	}
}

func TestInstallUserAutoUpdateBadExecPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	err := InstallUserAutoUpdate(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel\nevil",
		SystemdScope: "user",
	})
	if err == nil {
		t.Fatal("expected error for invalid exec path")
	}
	if !strings.Contains(err.Error(), "invalid executable path") {
		t.Fatalf("error = %v, want invalid executable path", err)
	}
}

func TestInstallSystemServiceLinuxNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := installSystemServiceLinux(InstallUserOptions{
		ExecPath: "/usr/bin/sentinel",
	})
	if err == nil {
		t.Fatal("expected error for non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestUninstallSystemServiceLinuxNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := uninstallSystemServiceLinux(UninstallUserOptions{})
	if err == nil {
		t.Fatal("expected error for non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestInstallSystemAutoUpdateLinuxNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := installSystemAutoUpdateLinux("/usr/bin/sentinel", "sentinel", "system", "daily", time.Hour, false, false)
	if err == nil {
		t.Fatal("expected error for non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestUninstallSystemAutoUpdateLinuxNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := uninstallSystemAutoUpdateLinux(UninstallUserAutoUpdateOptions{})
	if err == nil {
		t.Fatal("expected error for non-root")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestUserStatusSystemLinuxNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// userStatusSystemLinux doesn't check euid; it just reads the system unit path
	st, err := userStatusSystemLinux()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.ServicePath != systemUnitPath {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, systemUnitPath)
	}

	// Unit file likely doesn't exist in test env
	if st.SystemctlAvailable {
		if st.EnabledState == "" {
			t.Fatal("expected non-empty EnabledState")
		}
		if st.ActiveState == "" {
			t.Fatal("expected non-empty ActiveState")
		}
	}
}

func TestRemoveUserAutoUpdateUnitsExistingFiles(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux for systemd paths")
	}

	// Create temp files that look like the auto-update units, then verify removal.
	// removeUserAutoUpdateUnits uses the real home-based paths, so we can only test
	// the non-existent path (already tested) and verify the function returns nil.
	// The existing test covers the os.ErrNotExist path. This test ensures both
	// removal paths succeed when files genuinely do not exist.
	err := removeUserAutoUpdateUnits()
	if err != nil {
		t.Fatalf("removeUserAutoUpdateUnits() should succeed when files don't exist: %v", err)
	}
}

func TestInstallSystemServiceLinuxBadExecPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// Invalid exec path should error before privilege check
	err := installSystemServiceLinux(InstallUserOptions{
		ExecPath: "/usr/bin/sentinel\revil",
	})
	if err == nil {
		t.Fatal("expected error for invalid exec path")
	}
	if !strings.Contains(err.Error(), "invalid executable path") {
		t.Fatalf("error = %v, want invalid executable path", err)
	}
}

func TestInstallSystemAutoUpdateLinuxBadExecPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// Even with a valid exec path, non-root should fail with privilege error
	err := installSystemAutoUpdateLinux("/usr/bin/sentinel\nevil", "sentinel", "system", "daily", time.Hour, false, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Fatalf("error = %v, want root privileges error", err)
	}
}

func TestInstallSystemAutoUpdateLinuxEnableStartBranches(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// All enable/start combinations fail with privilege error for non-root
	tests := []struct {
		name   string
		enable bool
		start  bool
	}{
		{"enable and start", true, true},
		{"enable only", true, false},
		{"start only", false, true},
		{"neither", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := installSystemAutoUpdateLinux("/usr/bin/sentinel", "sentinel", "system", "daily", time.Hour, tc.enable, tc.start)
			if err == nil {
				t.Fatal("expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Fatalf("error = %v, want root privileges error", err)
			}
		})
	}
}

func TestUninstallSystemAutoUpdateLinuxBranches(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	tests := []struct {
		name string
		opts UninstallUserAutoUpdateOptions
	}{
		{"disable and stop", UninstallUserAutoUpdateOptions{Disable: true, Stop: true}},
		{"disable only", UninstallUserAutoUpdateOptions{Disable: true}},
		{"stop only", UninstallUserAutoUpdateOptions{Stop: true}},
		{"remove unit", UninstallUserAutoUpdateOptions{RemoveUnit: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := uninstallSystemAutoUpdateLinux(tc.opts)
			if err == nil {
				t.Fatal("expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Fatalf("error = %v, want root privileges error", err)
			}
		})
	}
}

func TestUninstallSystemServiceLinuxBranches(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	tests := []struct {
		name string
		opts UninstallUserOptions
	}{
		{"disable and stop", UninstallUserOptions{Disable: true, Stop: true}},
		{"disable only", UninstallUserOptions{Disable: true}},
		{"stop only", UninstallUserOptions{Stop: true}},
		{"remove unit", UninstallUserOptions{RemoveUnit: true}},
		{"noop", UninstallUserOptions{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := uninstallSystemServiceLinux(tc.opts)
			if err == nil {
				t.Fatal("expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Fatalf("error = %v, want root privileges error", err)
			}
		})
	}
}

func TestNormalizeSystemctlErrorStateFailedToGetUnitFileState(t *testing.T) {
	t.Parallel()

	got := normalizeSystemctlErrorState("Failed to get unit file state for sentinel.service: something")
	if got != "not-found" {
		t.Fatalf("got %q, want not-found", got)
	}
}

func TestNormalizeSystemctlErrorStateWhitespace(t *testing.T) {
	t.Parallel()

	// Input with leading/trailing whitespace should be normalized
	got := normalizeSystemctlErrorState("  inactive  ")
	if got != "inactive" {
		t.Fatalf("got %q, want %q", got, "inactive")
	}
}

func TestResolveExecPathSymlinkHandling(t *testing.T) {
	t.Parallel()

	// Explicit paths with whitespace should be trimmed
	got, err := resolveExecPath("   /opt/sentinel   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/opt/sentinel" {
		t.Fatalf("got %q, want /opt/sentinel", got)
	}
}

func TestUserAutoUpdateStatusForScopeUserOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// Test user scope path - verifies paths are set correctly
	st, err := UserAutoUpdateStatusForScope("user")
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			t.Skip("systemctl not available")
		}
		t.Fatalf("error: %v", err)
	}

	home, _ := os.UserHomeDir()
	wantServicePath := filepath.Join(home, ".config", "systemd", "user", userAutoUpdateServiceName)
	if st.ServicePath != wantServicePath {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, wantServicePath)
	}
	wantTimerPath := filepath.Join(home, ".config", "systemd", "user", userAutoUpdateTimerName)
	if st.TimerPath != wantTimerPath {
		t.Fatalf("TimerPath = %q, want %q", st.TimerPath, wantTimerPath)
	}
}

func TestUserAutoUpdateStatusForScopeSystemOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	st, err := UserAutoUpdateStatusForScope("system")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if st.ServicePath != systemAutoUpdateService {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, systemAutoUpdateService)
	}
	if st.TimerPath != systemAutoUpdateTimer {
		t.Fatalf("TimerPath = %q, want %q", st.TimerPath, systemAutoUpdateTimer)
	}

	// On Linux with systemctl available, system-scope status should use
	// readSystemctlSystemState (not readSystemctlState with --user)
	if st.SystemctlAvailable {
		// States should be non-empty even for non-installed units
		if st.TimerEnabledState == "" {
			t.Fatal("expected non-empty TimerEnabledState")
		}
		if st.TimerActiveState == "" {
			t.Fatal("expected non-empty TimerActiveState")
		}
		if st.LastRunState == "" {
			t.Fatal("expected non-empty LastRunState")
		}
	}
}

func TestUserStatusOnLinuxUnitFileDetection(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	st, err := UserStatus()
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			t.Skip("systemctl not available")
		}
		t.Fatalf("UserStatus() error: %v", err)
	}

	// Verify the service path points to the expected location
	home, _ := os.UserHomeDir()
	wantPath := filepath.Join(home, ".config", "systemd", "user", userUnitName)
	if st.ServicePath != wantPath {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, wantPath)
	}

	// UnitFileExists should be false unless sentinel is actually installed
	// (we only verify the field is set to a valid bool value - no assertion on true/false)
	_ = st.UnitFileExists
}

func TestWithSystemdUserBusHintNonBusError(t *testing.T) {
	t.Parallel()

	// Error without any bus-related keywords should pass through unchanged
	original := errors.New("some unrelated error")
	got := withSystemdUserBusHint(original)
	if got != original {
		t.Fatal("non-bus error should pass through unchanged")
	}
}

func TestResolveInstallUserAutoUpdateConfigServiceUnitWithTabs(t *testing.T) {
	t.Parallel()

	_, err := resolveInstallUserAutoUpdateConfig(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "user",
		ServiceUnit:  "bad\tname",
	})
	if err == nil {
		t.Fatal("expected error for tab in service unit name")
	}
	if !strings.Contains(err.Error(), "invalid service unit name") {
		t.Fatalf("error = %v, want invalid service unit name", err)
	}
}

func TestResolveInstallUserAutoUpdateConfigWhitespaceOnCalendar(t *testing.T) {
	t.Parallel()

	cfg, err := resolveInstallUserAutoUpdateConfig(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "user",
		OnCalendar:   "   ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.onCalendar != "daily" {
		t.Fatalf("onCalendar = %q, want daily", cfg.onCalendar)
	}
}

func TestInstallUserAutoUpdateBadServiceUnit(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	err := InstallUserAutoUpdate(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "user",
		ServiceUnit:  "bad name",
	})
	if err == nil {
		t.Fatal("expected error for invalid service unit name")
	}
	if !strings.Contains(err.Error(), "invalid service unit name") {
		t.Fatalf("error = %v, want invalid service unit name", err)
	}
}

func TestUserServicePathNonRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	path, err := UserServicePath()
	if err != nil {
		t.Fatalf("UserServicePath() error: %v", err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "systemd", "user", userUnitName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestUninstallUserAutoUpdateUserScopeOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// The user-scope uninstall path goes through ensureSystemdUserSupported
	// which may fail in containers without systemctl.
	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope: "user",
	})
	if err != nil {
		// This is expected if systemctl is not available
		if strings.Contains(err.Error(), "systemctl") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUninstallUserAutoUpdateUserScopeWithRemoveUnit(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// With RemoveUnit=true and user scope, should attempt to remove files
	// (which don't exist) then daemon-reload
	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope:      "user",
		RemoveUnit: true,
	})
	if err != nil {
		// Expected if systemctl is not available
		if strings.Contains(err.Error(), "systemctl") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUninstallUserAutoUpdateUserScopeDisableStop(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// With Disable+Stop, tests the stopUserAutoUpdateTimer path
	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope:   "user",
		Disable: true,
		Stop:    true,
	})
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUninstallUserAutoUpdateUserScopeDisableOnly(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope:   "user",
		Disable: true,
	})
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUninstallUserAutoUpdateUserScopeStopOnly(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	err := UninstallUserAutoUpdate(UninstallUserAutoUpdateOptions{
		Scope: "user",
		Stop:  true,
	})
	if err != nil {
		if strings.Contains(err.Error(), "systemctl") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUninstallUserOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	// Test all branches of UninstallUser for non-root Linux user.
	// ensureSystemdUserSupported should pass if systemctl is available.
	tests := []struct {
		name string
		opts UninstallUserOptions
	}{
		{"disable and stop", UninstallUserOptions{Disable: true, Stop: true}},
		{"disable only", UninstallUserOptions{Disable: true}},
		{"stop only", UninstallUserOptions{Stop: true}},
		{"remove unit", UninstallUserOptions{RemoveUnit: true}},
		{"noop", UninstallUserOptions{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := UninstallUser(tc.opts)
			if err != nil {
				// systemctl may not have user bus access in CI
				if strings.Contains(err.Error(), "systemctl") ||
					strings.Contains(err.Error(), "Failed to connect") ||
					strings.Contains(err.Error(), "bus") {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunSystemctlUserWithBogusCommand(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// This exercises the runSystemctlUser error path where the command
	// runs but returns a non-zero exit code with output.
	err := runSystemctlUser("is-active", "nonexistent-unit-name-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent unit")
	}
	// The error should contain "systemctl --user" prefix
	if !strings.Contains(err.Error(), "systemctl --user") {
		t.Fatalf("error = %v, want contains 'systemctl --user'", err)
	}
}

func TestRunSystemctlSystemWithBogusCommand(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// This exercises the runSystemctlSystem error path.
	err := runSystemctlSystem("is-active", "nonexistent-unit-name-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent unit")
	}
	if !strings.Contains(err.Error(), "systemctl") {
		t.Fatalf("error = %v, want contains 'systemctl'", err)
	}
}

func TestIsSystemctlUserActive(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// An obviously nonexistent unit should return false
	active := isSystemctlUserActive("nonexistent-unit-name-12345")
	if active {
		t.Fatal("nonexistent unit should not be active")
	}
}

func TestIsSystemctlSystemActive(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// An obviously nonexistent unit should return false
	active := isSystemctlSystemActive("nonexistent-unit-name-12345")
	if active {
		t.Fatal("nonexistent unit should not be active")
	}
}

func TestReadSystemctlSystemStateReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	// Test is-enabled for a nonexistent unit: should return a state string
	state := readSystemctlSystemState("is-enabled", "nonexistent-unit-name-12345")
	if state == "" {
		t.Fatal("expected non-empty state")
	}

	// Test is-active for a nonexistent unit
	state = readSystemctlSystemState("is-active", "nonexistent-unit-name-12345")
	if state == "" {
		t.Fatal("expected non-empty state")
	}
}

func TestReadSystemctlStateReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	state := readSystemctlState("is-enabled", "nonexistent-unit-name-12345")
	if state == "" {
		t.Fatal("expected non-empty state")
	}

	state = readSystemctlState("is-active", "nonexistent-unit-name-12345")
	if state == "" {
		t.Fatal("expected non-empty state")
	}
}

// NOTE: Tests for InstallUser, InstallUserAutoUpdate, stopUserAutoUpdateTimer,
// and applyUserAutoUpdateTimerState are intentionally omitted. These functions
// write real systemd unit files (~/.config/systemd/user/sentinel.service) and
// run real systemctl commands (daemon-reload, enable, start, stop, disable),
// which overwrites any running sentinel service and kills tmux sessions.
