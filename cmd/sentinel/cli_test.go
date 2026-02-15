package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/service"
	"github.com/opus-domini/sentinel/internal/updater"
)

const (
	testSentinelPath    = "/tmp/sentinel"
	testCurrentVersion1 = "1.0.0"
	testScopeUser       = "user"
	testScopeSystem     = "system"
)

func TestRunCLIDefaultServe(t *testing.T) {
	origServe := serveFn
	t.Cleanup(func() { serveFn = origServe })

	called := false
	serveFn = func() int {
		called = true
		return 0
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !called {
		t.Fatal("serveFn was not called")
	}
}

func TestRunCLIServiceInstallParsesFlags(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	var got service.InstallUserOptions
	installUserSvcFn = func(opts service.InstallUserOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "install", "--exec", testSentinelPath, "--enable=false", "--start=false"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if got.Enable {
		t.Fatal("Enable = true, want false")
	}
	if got.Start {
		t.Fatal("Start = true, want false")
	}
}

func TestRunCLIServiceStatus(t *testing.T) {
	origStatus := userStatusFn
	t.Cleanup(func() { userStatusFn = origStatus })

	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/tmp/sentinel.service",
			UnitFileExists:     true,
			SystemctlAvailable: true,
			EnabledState:       "enabled",
			ActiveState:        "active",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	managerLabel := runtimeServiceManagerLabel()
	for _, fragment := range []string{
		"user unit file: /tmp/sentinel.service",
		"user unit exists: true",
		managerLabel + " available: true",
		"user unit enabled: enabled",
		"user unit active: active",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIServiceStatusSystemUnitLabel(t *testing.T) {
	origStatus := userStatusFn
	t.Cleanup(func() { userStatusFn = origStatus })

	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/etc/systemd/system/sentinel.service",
			UnitFileExists:     false,
			SystemctlAvailable: true,
			EnabledState:       "not-found",
			ActiveState:        "inactive",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	managerLabel := runtimeServiceManagerLabel()
	for _, fragment := range []string{
		"system unit file: /etc/systemd/system/sentinel.service",
		"system unit exists: false",
		managerLabel + " available: true",
		"system unit enabled: not-found",
		"system unit active: inactive",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIDoctor(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := userStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		userStatusFn = origStatus
	})

	loadConfigFn = func() config.Config {
		return config.Config{
			ListenAddr: "127.0.0.1:4040",
			DataDir:    "/tmp/.sentinel",
			Token:      "token",
		}
	}
	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/tmp/sentinel.service",
			UnitFileExists:     true,
			EnabledState:       "enabled",
			ActiveState:        "active",
			SystemctlAvailable: true,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"Sentinel doctor report",
		"listen: 127.0.0.1:4040",
		"data dir: /tmp/.sentinel",
		"token required: true",
		"user unit file: /tmp/sentinel.service",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIDoctorSystemUnitLabel(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := userStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		userStatusFn = origStatus
	})

	loadConfigFn = func() config.Config {
		return config.Config{
			ListenAddr: "127.0.0.1:4040",
			DataDir:    "/tmp/.sentinel",
			Token:      "",
		}
	}
	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/etc/systemd/system/sentinel.service",
			UnitFileExists:     false,
			EnabledState:       "not-found",
			ActiveState:        "inactive",
			SystemctlAvailable: true,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"system unit file: /etc/systemd/system/sentinel.service",
		"system unit exists: false",
		"system unit enabled: not-found",
		"system unit active: inactive",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"unknown"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command: unknown") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIServiceInstallFailure(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	installUserSvcFn = func(_ service.InstallUserOptions) error {
		return errors.New("install failed")
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "install"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service install failed") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIHelpFlag(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"-h"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Sentinel command-line interface") {
		t.Fatalf("unexpected help output: %s", out.String())
	}
}

func TestRunCLIServiceAutoUpdateInstallParsesFlags(t *testing.T) {
	origInstall := installUserAutoUpdateFn
	t.Cleanup(func() { installUserAutoUpdateFn = origInstall })

	var got service.InstallUserAutoUpdateOptions
	installUserAutoUpdateFn = func(opts service.InstallUserAutoUpdateOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{
		"service", "autoupdate", "install",
		"--exec", testSentinelPath,
		"--enable=false",
		"--start=false",
		"--service", "sentinel-custom",
		"--scope", testScopeSystem,
		"--on-calendar", "hourly",
		"--randomized-delay", "30m",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if got.Enable {
		t.Fatal("Enable = true, want false")
	}
	if got.Start {
		t.Fatal("Start = true, want false")
	}
	if got.ServiceUnit != "sentinel-custom" {
		t.Fatalf("ServiceUnit = %q, want sentinel-custom", got.ServiceUnit)
	}
	if got.SystemdScope != testScopeSystem {
		t.Fatalf("SystemdScope = %q, want %s", got.SystemdScope, testScopeSystem)
	}
	if got.OnCalendar != "hourly" {
		t.Fatalf("OnCalendar = %q, want hourly", got.OnCalendar)
	}
	if got.RandomizedDelay != 30*time.Minute {
		t.Fatalf("RandomizedDelay = %s, want 30m", got.RandomizedDelay)
	}
}

func TestRunCLIServiceAutoUpdateStatus(t *testing.T) {
	origStatus := userAutoUpdateStatusFn
	t.Cleanup(func() { userAutoUpdateStatusFn = origStatus })

	userAutoUpdateStatusFn = func(scope string) (service.UserAutoUpdateServiceStatus, error) {
		if scope != testScopeUser {
			t.Fatalf("scope = %q, want %s", scope, testScopeUser)
		}
		return service.UserAutoUpdateServiceStatus{
			ServicePath:        "/tmp/sentinel-updater.service",
			TimerPath:          "/tmp/sentinel-updater.timer",
			ServiceUnitExists:  true,
			TimerUnitExists:    true,
			SystemctlAvailable: true,
			TimerEnabledState:  "enabled",
			TimerActiveState:   "active",
			LastRunState:       "inactive",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "autoupdate", "status", "--scope", testScopeUser}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"service file: /tmp/sentinel-updater.service",
		"timer file: /tmp/sentinel-updater.timer",
		"timer enabled: enabled",
		"timer active: active",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIServiceAutoUpdateStatusScopeFlag(t *testing.T) {
	origStatus := userAutoUpdateStatusFn
	t.Cleanup(func() { userAutoUpdateStatusFn = origStatus })

	userAutoUpdateStatusFn = func(scope string) (service.UserAutoUpdateServiceStatus, error) {
		if scope != testScopeSystem {
			t.Fatalf("scope = %q, want %s", scope, testScopeSystem)
		}
		return service.UserAutoUpdateServiceStatus{}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "autoupdate", "status", "--scope", testScopeSystem}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
}

func TestRunCLIServiceAutoUpdateUninstallScopeFlag(t *testing.T) {
	origUninstall := uninstallUserAutoUpdateFn
	t.Cleanup(func() { uninstallUserAutoUpdateFn = origUninstall })

	var got service.UninstallUserAutoUpdateOptions
	uninstallUserAutoUpdateFn = func(opts service.UninstallUserAutoUpdateOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "autoupdate", "uninstall", "--scope", testScopeSystem}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.Scope != testScopeSystem {
		t.Fatalf("Scope = %q, want %s", got.Scope, testScopeSystem)
	}
}

func TestRunCLIUpdateCheck(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origCheck := updateCheckFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateCheckFn = origCheck
	})

	loadConfigFn = func() config.Config {
		return config.Config{DataDir: testSentinelPath}
	}
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.CheckOptions
	updateCheckFn = func(_ context.Context, opts updater.CheckOptions) (updater.CheckResult, error) {
		got = opts
		return updater.CheckResult{
			CurrentVersion: "1.0.0",
			LatestVersion:  "1.1.0",
			UpToDate:       false,
			ReleaseURL:     "https://github.com/opus-domini/sentinel/releases/tag/v1.1.0",
			AssetName:      "sentinel-1.1.0-linux-amd64.tar.gz",
			ExpectedSHA256: strings.Repeat("a", 64),
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"update", "check", "--repo", "opus-domini/sentinel", "--api", "http://example", "--os", "linux", "--arch", "amd64"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.CurrentVersion != testCurrentVersion1 {
		t.Fatalf("CurrentVersion = %q, want %s", got.CurrentVersion, testCurrentVersion1)
	}
	if got.DataDir != testSentinelPath {
		t.Fatalf("DataDir = %q, want %s", got.DataDir, testSentinelPath)
	}
	if got.Repo != "opus-domini/sentinel" {
		t.Fatalf("Repo = %q, want opus-domini/sentinel", got.Repo)
	}
	if !strings.Contains(out.String(), "latest version: 1.1.0") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunCLIUpdateApplyParsesFlags(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = func() config.Config {
		return config.Config{DataDir: testSentinelPath}
	}
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.ApplyOptions
	updateApplyFn = func(_ context.Context, opts updater.ApplyOptions) (updater.ApplyResult, error) {
		got = opts
		return updater.ApplyResult{
			Applied:        true,
			CurrentVersion: testCurrentVersion1,
			LatestVersion:  "1.1.0",
			BinaryPath:     testSentinelPath,
			BackupPath:     testSentinelPath + ".bak",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{
		"update", "apply",
		"--repo", "opus-domini/sentinel",
		"--api", "http://example",
		"--os", "linux",
		"--arch", "amd64",
		"--exec", testSentinelPath,
		"--allow-downgrade=true",
		"--allow-unverified=true",
		"--restart=true",
		"--service", "sentinel",
		"--scope", "user",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.DataDir != testSentinelPath {
		t.Fatalf("DataDir = %q, want %s", got.DataDir, testSentinelPath)
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if !got.AllowDowngrade {
		t.Fatal("AllowDowngrade = false, want true")
	}
	if !got.AllowUnverified {
		t.Fatal("AllowUnverified = false, want true")
	}
	if !got.Restart {
		t.Fatal("Restart = false, want true")
	}
	if !strings.Contains(out.String(), "updated from: 1.0.0") || !strings.Contains(out.String(), "updated to: 1.1.0") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunCLIUpdateApplyParsesLegacySystemdScopeFlag(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = func() config.Config {
		return config.Config{DataDir: testSentinelPath}
	}
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.ApplyOptions
	updateApplyFn = func(_ context.Context, opts updater.ApplyOptions) (updater.ApplyResult, error) {
		got = opts
		return updater.ApplyResult{
			Applied:        false,
			CurrentVersion: testCurrentVersion1,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"update", "apply", "--systemd-scope", "system"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.SystemdScope != testScopeSystem {
		t.Fatalf("SystemdScope = %q, want %s", got.SystemdScope, testScopeSystem)
	}
}

func TestRunCLIUpdateApplyRejectsConflictingScopeFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"update", "apply", "--scope", testScopeUser, "--systemd-scope", testScopeSystem}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "invalid scope flags") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIUpdateStatus(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := updateStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		updateStatusFn = origStatus
	})

	loadConfigFn = func() config.Config {
		return config.Config{DataDir: testSentinelPath}
	}
	updateStatusFn = func(dataDir string) (updater.State, error) {
		if dataDir != testSentinelPath {
			t.Fatalf("dataDir = %q, want %s", dataDir, testSentinelPath)
		}
		return updater.State{
			LastCheckedAt:  time.Date(2026, time.February, 15, 12, 0, 0, 0, time.UTC),
			LastAppliedAt:  time.Date(2026, time.February, 15, 12, 30, 0, 0, time.UTC),
			CurrentVersion: "1.1.0",
			LatestVersion:  "1.1.0",
			UpToDate:       true,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"update", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"current version: 1.1.0",
		"latest version: 1.1.0",
		"up to date: true",
		"last checked: 2026-02-15T12:00:00Z",
		"last applied: 2026-02-15T12:30:00Z",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIVersionFlag(t *testing.T) {
	origVersion := currentVersionFn
	t.Cleanup(func() { currentVersionFn = origVersion })
	currentVersionFn = func() string { return "v1.2.3" }

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != "sentinel version v1.2.3" {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestCurrentVersionPrefersBuildVersion(t *testing.T) {
	orig := buildVersion
	t.Cleanup(func() { buildVersion = orig })

	buildVersion = "1.9.0"
	if got := currentVersion(); got != "1.9.0" {
		t.Fatalf("currentVersion() = %q, want 1.9.0", got)
	}
}

func TestDefaultAutoUpdateScopeFlag(t *testing.T) {
	t.Parallel()

	if got := defaultAutoUpdateScopeFlag(); got != "auto" {
		t.Fatalf("defaultAutoUpdateScopeFlag() = %q, want auto", got)
	}
}
