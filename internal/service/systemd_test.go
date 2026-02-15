package service

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
				"sentinel",
				tc.enable,
				tc.start,
				func(unit string) bool {
					if unit != "sentinel" {
						t.Fatalf("unit = %q, want sentinel", unit)
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
		"sentinel",
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
		"sentinel",
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
