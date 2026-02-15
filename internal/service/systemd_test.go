package service

import (
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
