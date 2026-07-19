package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/config"
)

func TestSelectDeploymentMatrix(t *testing.T) {
	t.Parallel()

	user := Deployment{Scope: ScopeUser, BinaryPath: "/user/sentinel"}
	system := Deployment{Scope: ScopeSystem, BinaryPath: "/system/sentinel"}
	tests := []struct {
		name        string
		installed   []Deployment
		scope       string
		wantScope   string
		wantErr     error
		wantMessage string
	}{
		{name: "none auto", scope: ScopeAuto, wantErr: ErrNoServiceInstalled},
		{name: "user auto", installed: []Deployment{user}, scope: ScopeAuto, wantScope: ScopeUser},
		{name: "system auto", installed: []Deployment{system}, scope: ScopeAuto, wantScope: ScopeSystem},
		{name: "both auto", installed: []Deployment{user, system}, scope: ScopeAuto, wantErr: ErrAmbiguousDeployment},
		{name: "both explicit user", installed: []Deployment{user, system}, scope: ScopeUser, wantScope: ScopeUser},
		{name: "both explicit system", installed: []Deployment{user, system}, scope: ScopeSystem, wantScope: ScopeSystem},
		{name: "missing explicit scope", installed: []Deployment{user}, scope: ScopeSystem, wantMessage: "no Sentinel service is installed in system scope"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectDeployment(tc.installed, tc.scope)
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMessage != "" && (err == nil || !strings.Contains(err.Error(), tc.wantMessage)) {
				t.Fatalf("error = %v, want message %q", err, tc.wantMessage)
			}
			if tc.wantErr == nil && tc.wantMessage == "" {
				if err != nil {
					t.Fatalf("selectDeployment() error = %v", err)
				}
				if got.Scope != tc.wantScope {
					t.Fatalf("scope = %q, want %q", got.Scope, tc.wantScope)
				}
			}
		})
	}
}

func TestSelectAccessibleDeploymentDiagnosesScopeBeforeUnitRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		installed []Deployment
		scope     string
		euid      int
		wantScope string
		wantError string
	}{
		{
			name:      "unprivileged caller with system deployment",
			installed: []Deployment{{Scope: ScopeSystem}},
			scope:     ScopeAuto,
			euid:      1000,
			wantError: "deployment is system-wide; re-run with sudo",
		},
		{
			name:      "root caller with user deployment",
			installed: []Deployment{{Scope: ScopeUser}},
			scope:     ScopeAuto,
			euid:      0,
			wantError: "deployment belongs to a user",
		},
		{
			name:      "matching user caller",
			installed: []Deployment{{Scope: ScopeUser}},
			scope:     ScopeAuto,
			euid:      1000,
			wantScope: ScopeUser,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectAccessibleDeployment(tc.installed, tc.scope, tc.euid)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectAccessibleDeployment() error = %v", err)
			}
			if got.Scope != tc.wantScope {
				t.Fatalf("scope = %q, want %q", got.Scope, tc.wantScope)
			}
		})
	}
}

func TestSelectInstallScopeMatrix(t *testing.T) {
	t.Parallel()

	user := Deployment{Scope: ScopeUser}
	system := Deployment{Scope: ScopeSystem}
	tests := []struct {
		name      string
		installed []Deployment
		scope     string
		want      string
		wantErr   error
	}{
		{name: "fresh auto requires choice", scope: ScopeAuto, wantErr: ErrInstallScopeRequired},
		{name: "fresh explicit user", scope: ScopeUser, want: ScopeUser},
		{name: "fresh explicit system", scope: ScopeSystem, want: ScopeSystem},
		{name: "preserve user", installed: []Deployment{user}, scope: ScopeAuto, want: ScopeUser},
		{name: "preserve system", installed: []Deployment{system}, scope: ScopeAuto, want: ScopeSystem},
		{name: "reject user to system", installed: []Deployment{user}, scope: ScopeSystem, wantErr: ErrAmbiguousDeployment},
		{name: "reject system to user", installed: []Deployment{system}, scope: ScopeUser, wantErr: ErrAmbiguousDeployment},
		{name: "reject both", installed: []Deployment{user, system}, scope: ScopeAuto, wantErr: ErrAmbiguousDeployment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectInstallScope(tc.installed, tc.scope)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("selectInstallScope() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectInstallScope() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("scope = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInstallationCandidatesPreferManagedServices(t *testing.T) {
	managed := []Deployment{{Scope: ScopeSystem, BinaryPath: "/opt/sentinel"}}
	got := installationCandidates(managed, true, true, "/home/user/.local/bin/sentinel", "/usr/local/bin/sentinel")
	if len(got) != 1 || got[0].BinaryPath != "/opt/sentinel" {
		t.Fatalf("installationCandidates() = %#v", got)
	}
}

func TestInstallationCandidatesUseCanonicalStandaloneBinaries(t *testing.T) {
	got := installationCandidates(nil, true, true, "/home/user/.local/bin/sentinel", "/usr/local/bin/sentinel")
	if len(got) != 2 || got[0].Scope != ScopeUser || got[1].Scope != ScopeSystem {
		t.Fatalf("installationCandidates() = %#v", got)
	}
}

func TestCanonicalBinaryPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	userPath, err := CanonicalBinaryPath(ScopeUser)
	if err != nil || !strings.HasSuffix(userPath, filepath.Join(".local", "bin", "sentinel")) {
		t.Fatalf("CanonicalBinaryPath(user) = %q, %v", userPath, err)
	}
	systemPath, err := CanonicalBinaryPath(ScopeSystem)
	if err != nil || systemPath != "/usr/local/bin/sentinel" {
		t.Fatalf("CanonicalBinaryPath(system) = %q, %v", systemPath, err)
	}
	if _, err := CanonicalBinaryPath("invalid"); err == nil {
		t.Fatal("CanonicalBinaryPath(invalid) error = nil")
	}
}

func TestRegularFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(file, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !regularFileExists(file) {
		t.Fatal("regularFileExists() rejected a regular file")
	}
	if regularFileExists(dir) || regularFileExists(filepath.Join(dir, "missing")) {
		t.Fatal("regularFileExists() accepted a directory or missing path")
	}
}

func TestParseSystemdDeploymentFields(t *testing.T) {
	t.Parallel()

	unit := renderUserUnit("/opt/Sentinel App/sentinel", "/etc/sentinel/config.toml", "/var/lib/sentinel", "/var/log/sentinel/sentinel.log")
	binary, configPath := parseSystemdExecStart(unit)
	if binary != "/opt/Sentinel App/sentinel" {
		t.Fatalf("binary = %q", binary)
	}
	if configPath != "/etc/sentinel/config.toml" {
		t.Fatalf("config = %q", configPath)
	}
	if got := parseSystemdEnvironment(unit, "SENTINEL_DATA_DIR"); got != "/var/lib/sentinel" {
		t.Fatalf("data dir = %q", got)
	}
	if got := parseSystemdEnvironment(unit, config.ManagedDefaultLogPathEnv); got != "/var/log/sentinel/sentinel.log" {
		t.Fatalf("default log = %q", got)
	}
}

func TestParseLaunchdDeploymentFields(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserServicePlist("/opt/sentinel", "/Library/Preferences/io.opusdomini.sentinel.toml", "/Library/Application Support/Sentinel", "/Library/Logs/Sentinel/sentinel.log", "/tmp/out", "/tmp/err")
	binary, configPath := parseLaunchdProgramArguments(plist)
	if binary != "/opt/sentinel" || configPath != "/Library/Preferences/io.opusdomini.sentinel.toml" {
		t.Fatalf("binary=%q config=%q", binary, configPath)
	}
	if got := parseLaunchdEnvironment(plist, "SENTINEL_DATA_DIR"); got != "/Library/Application Support/Sentinel" {
		t.Fatalf("data dir = %q", got)
	}
	if got := parseLaunchdEnvironment(plist, config.ManagedDefaultLogPathEnv); got != "/Library/Logs/Sentinel/sentinel.log" {
		t.Fatalf("default log = %q", got)
	}
}

func TestParseDeploymentMissingFields(t *testing.T) {
	t.Parallel()

	if binary, configPath := parseSystemdExecStart("[Service]\nType=simple\n"); binary != "" || configPath != "" {
		t.Fatalf("unexpected systemd fields: binary=%q config=%q", binary, configPath)
	}
	if binary, configPath := parseSystemdExecStart("ExecStart=\n"); binary != "" || configPath != "" {
		t.Fatalf("unexpected empty ExecStart fields: binary=%q config=%q", binary, configPath)
	}
	if got := parseSystemdEnvironment("Environment=OTHER=value\n", "SENTINEL_DATA_DIR"); got != "" {
		t.Fatalf("unexpected systemd environment = %q", got)
	}
	if binary, configPath := parseLaunchdProgramArguments("<plist></plist>"); binary != "" || configPath != "" {
		t.Fatalf("unexpected launchd fields: binary=%q config=%q", binary, configPath)
	}
	if binary, configPath := parseLaunchdProgramArguments("<key>ProgramArguments</key><array>"); binary != "" || configPath != "" {
		t.Fatalf("unexpected unterminated launchd fields: binary=%q config=%q", binary, configPath)
	}
	if got := parseLaunchdEnvironment("<plist></plist>", "SENTINEL_DATA_DIR"); got != "" {
		t.Fatalf("unexpected launchd environment = %q", got)
	}
}

func TestDeploymentPathHelpers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	userConfig, userData, err := ScopePaths(ScopeUser)
	if err != nil {
		t.Fatalf("ScopePaths(user) error = %v", err)
	}
	if userConfig != filepath.Join(userData, "config.toml") {
		t.Fatalf("user config = %q, data = %q", userConfig, userData)
	}

	systemConfig, systemData, err := ScopePaths(ScopeSystem)
	if err != nil {
		t.Fatalf("ScopePaths(system) error = %v", err)
	}
	if runtime.GOOS == launchdSupportedOS {
		if systemData != "/Library/Application Support/Sentinel" {
			t.Fatalf("system data = %q", systemData)
		}
	} else if systemConfig != "/etc/sentinel/config.toml" || systemData != "/var/lib/sentinel" {
		t.Fatalf("system config = %q, data = %q", systemConfig, systemData)
	}
	systemLayout, err := LayoutForScope(ScopeSystem)
	if err != nil {
		t.Fatalf("LayoutForScope(system) error = %v", err)
	}
	if runtime.GOOS == launchdSupportedOS {
		if systemLayout.ConfigPath != "/Library/Preferences/io.opusdomini.sentinel.toml" ||
			systemLayout.LogPath != "/Library/Logs/Sentinel/sentinel.log" {
			t.Fatalf("system layout = %+v", systemLayout)
		}
	} else if systemLayout.LogPath != "/var/log/sentinel/sentinel.log" {
		t.Fatalf("system log = %q", systemLayout.LogPath)
	}

	if _, _, err := ScopePaths("bogus"); err == nil {
		t.Fatal("ScopePaths() accepted an invalid scope")
	}
	legacyConfig, legacyData, err := legacyScopePaths(ScopeUser)
	if err != nil || legacyConfig != userConfig || legacyData != userData {
		t.Fatalf("legacy user paths = %q, %q, %v", legacyConfig, legacyData, err)
	}
	if _, _, err := legacyScopePaths(ScopeSystem); err != nil {
		t.Fatalf("legacyScopePaths(system) error = %v", err)
	}
	legacyLayout, err := LegacyLayoutForScope(ScopeSystem)
	if err != nil || legacyLayout.ConfigPath == "" || legacyLayout.DataDir == "" || legacyLayout.LogPath == "" {
		t.Fatalf("LegacyLayoutForScope(system) = %+v, %v", legacyLayout, err)
	}
	if canonical, err := HasCanonicalPaths(Deployment{
		Scope:      ScopeSystem,
		ConfigPath: systemLayout.ConfigPath,
		DataDir:    systemLayout.DataDir,
	}); err != nil || !canonical {
		t.Fatalf("HasCanonicalPaths(system) = %t, %v", canonical, err)
	}
	if _, err := servicePathForScope("bogus"); err == nil {
		t.Fatal("servicePathForScope() accepted an invalid scope")
	}
}

func TestAutoUpdateMigrationControlRejectsInvalidScope(t *testing.T) {
	t.Parallel()

	if _, err := PauseAutoUpdate("bogus"); err == nil {
		t.Fatal("PauseAutoUpdate() accepted invalid scope")
	}
	if err := ResumeAutoUpdate("bogus", true); err == nil {
		t.Fatal("ResumeAutoUpdate() accepted invalid scope")
	}
	if err := ResumeAutoUpdate("bogus", false); err != nil {
		t.Fatalf("inactive ResumeAutoUpdate() error = %v", err)
	}
}

func TestAutoUpdateMigrationControlPausesAndResumesUserTimer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("user-scope access intentionally rejects root")
	}
	t.Setenv("HOME", t.TempDir())
	origOS := autoUpdateControlOS
	origStatus := autoUpdateStatusFn
	origUserctl := autoUpdateRunUserctlFn
	t.Cleanup(func() {
		autoUpdateControlOS = origOS
		autoUpdateStatusFn = origStatus
		autoUpdateRunUserctlFn = origUserctl
	})
	autoUpdateControlOS = systemdSupportedOS
	autoUpdateStatusFn = func(string) (UserAutoUpdateServiceStatus, error) {
		return UserAutoUpdateServiceStatus{
			ServiceUnitExists: true,
			TimerUnitExists:   true,
			TimerActiveState:  "active",
		}, nil
	}
	var calls [][]string
	autoUpdateRunUserctlFn = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	active, err := PauseAutoUpdate(ScopeUser)
	if err != nil || !active {
		t.Fatalf("PauseAutoUpdate() = %t, %v", active, err)
	}
	if err := ResumeAutoUpdate(ScopeUser, active); err != nil {
		t.Fatalf("ResumeAutoUpdate() error = %v", err)
	}
	if len(calls) != 2 || calls[0][0] != "stop" || calls[1][0] != "start" {
		t.Fatalf("systemctl calls = %v", calls)
	}
	autoUpdateStatusFn = func(string) (UserAutoUpdateServiceStatus, error) {
		return UserAutoUpdateServiceStatus{}, nil
	}
	if active, err := PauseAutoUpdate(ScopeUser); err != nil || active {
		t.Fatalf("PauseAutoUpdate(no units) = %t, %v", active, err)
	}
	autoUpdateStatusFn = func(string) (UserAutoUpdateServiceStatus, error) {
		return UserAutoUpdateServiceStatus{}, errors.New("status unavailable")
	}
	if _, err := PauseAutoUpdate(ScopeUser); err == nil || !strings.Contains(err.Error(), "status unavailable") {
		t.Fatalf("PauseAutoUpdate(status error) = %v", err)
	}
}

func TestAutoUpdateMigrationControlPausesAndResumesLaunchdJob(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("user-scope access intentionally rejects root")
	}
	t.Setenv("HOME", t.TempDir())
	origOS := autoUpdateControlOS
	origStatus := autoUpdateStatusFn
	origBootout := autoUpdateBootoutFn
	origBootstrap := autoUpdateBootstrapFn
	t.Cleanup(func() {
		autoUpdateControlOS = origOS
		autoUpdateStatusFn = origStatus
		autoUpdateBootoutFn = origBootout
		autoUpdateBootstrapFn = origBootstrap
	})
	autoUpdateControlOS = launchdSupportedOS
	autoUpdateStatusFn = func(string) (UserAutoUpdateServiceStatus, error) {
		return UserAutoUpdateServiceStatus{
			ServiceUnitExists: true,
			TimerUnitExists:   true,
			TimerActiveState:  "active",
		}, nil
	}
	bootoutCalls := 0
	bootstrapCalls := 0
	autoUpdateBootoutFn = func(_, _ string) error {
		bootoutCalls++
		return nil
	}
	autoUpdateBootstrapFn = func(_, _, _ string) error {
		bootstrapCalls++
		return nil
	}
	active, err := PauseAutoUpdate(ScopeUser)
	if err != nil || !active {
		t.Fatalf("PauseAutoUpdate() = %t, %v", active, err)
	}
	if err := ResumeAutoUpdate(ScopeUser, active); err != nil {
		t.Fatalf("ResumeAutoUpdate() error = %v", err)
	}
	if bootoutCalls != 1 || bootstrapCalls != 1 {
		t.Fatalf("bootout=%d bootstrap=%d", bootoutCalls, bootstrapCalls)
	}
	autoUpdateStatusFn = func(string) (UserAutoUpdateServiceStatus, error) {
		return UserAutoUpdateServiceStatus{ServiceUnitExists: true, TimerActiveState: "inactive"}, nil
	}
	if active, err := PauseAutoUpdate(ScopeUser); err != nil || active {
		t.Fatalf("PauseAutoUpdate(inactive launchd) = %t, %v", active, err)
	}
}

func TestNormalizeExplicitScope(t *testing.T) {
	t.Parallel()

	for _, scope := range []string{ScopeUser, ScopeSystem} {
		got, err := normalizeExplicitScope(scope)
		if err != nil || got != scope {
			t.Fatalf("normalizeExplicitScope(%q) = %q, %v", scope, got, err)
		}
	}
	for _, scope := range []string{"", ScopeAuto, "bogus"} {
		if _, err := normalizeExplicitScope(scope); err == nil {
			t.Fatalf("normalizeExplicitScope(%q) accepted an invalid target", scope)
		}
	}
}

func TestManagedCommandsRejectInvalidScopeBeforeHostAccess(t *testing.T) {
	t.Parallel()

	if _, err := ResolveInstallScope("bogus"); err == nil {
		t.Fatal("ResolveInstallScope() accepted an invalid scope")
	}
	if err := InstallUser(InstallUserOptions{Scope: ScopeAuto}); err == nil {
		t.Fatal("InstallUser() accepted an automatic scope")
	}
	if err := UninstallUser(UninstallUserOptions{Scope: "bogus"}); err == nil {
		t.Fatal("UninstallUser() accepted an invalid scope")
	}
	if err := Control(actionStart, "bogus"); err == nil {
		t.Fatal("Control() accepted an invalid scope")
	}
}

func TestValidateExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	executable := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateExecutable(executable); err != nil {
		t.Fatalf("validateExecutable() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "relative", path: "sentinel", want: "must be absolute"},
		{name: "missing", path: filepath.Join(dir, "missing"), want: "inspect executable"},
		{name: "directory", path: dir, want: "not a regular file"},
	}
	notExecutable := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(notExecutable, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests = append(tests, struct {
		name string
		path string
		want string
	}{name: "not executable", path: notExecutable, want: "is not executable"})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExecutable(tc.path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
