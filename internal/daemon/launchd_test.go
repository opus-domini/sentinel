package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderLaunchdUserServicePlistIncludesExecStart(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserServicePlist("/usr/local/bin/sentinel", "/tmp/sentinel.out.log", "/tmp/sentinel.err.log")
	if !strings.Contains(plist, "<string>/usr/local/bin/sentinel</string>") {
		t.Fatalf("plist missing executable path: %s", plist)
	}
	if !strings.Contains(plist, "<string>serve</string>") {
		t.Fatalf("plist missing serve argument: %s", plist)
	}
	if !strings.Contains(plist, "<string>"+launchdServiceLabel+"</string>") {
		t.Fatalf("plist missing launchd label: %s", plist)
	}
}

func TestRenderLaunchdUserAutoUpdatePlistIncludesApplyArgs(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserAutoUpdatePlist(
		"/usr/local/bin/sentinel",
		launchdServiceLabel,
		managerScopeUser,
		86400,
		"/tmp/sentinel-updater.out.log",
		"/tmp/sentinel-updater.err.log",
	)
	for _, fragment := range []string{
		"<string>update</string>",
		"<string>apply</string>",
		"<string>-restart=true</string>",
		"<string>-service=" + launchdServiceLabel + "</string>",
		"<string>-systemd-scope=" + managerScopeUser + "</string>",
		"<integer>86400</integer>",
	} {
		if !strings.Contains(plist, fragment) {
			t.Fatalf("plist missing %q: %s", fragment, plist)
		}
	}
}

func TestLaunchdStartInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  int
	}{
		{input: "", want: 86400},
		{input: "daily", want: 86400}, //nolint:goconst // test value
		{input: "hourly", want: 3600},
		{input: "weekly", want: 604800},
		{input: "30m", want: 1800},
		{input: "300", want: 300},
	}
	for _, tc := range cases {
		got, err := launchdStartInterval(tc.input)
		if err != nil {
			t.Fatalf("launchdStartInterval(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("launchdStartInterval(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestLaunchdStartIntervalRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	if _, err := launchdStartInterval("invalid"); err == nil {
		t.Fatal("expected error for invalid launchd interval")
	}
}

func TestLaunchdLabelFromServiceUnit(t *testing.T) {
	t.Parallel()

	label, err := launchdLabelFromServiceUnit("")
	if err != nil {
		t.Fatalf("launchdLabelFromServiceUnit(\"\") error: %v", err)
	}
	if label != launchdServiceLabel {
		t.Fatalf("default label = %q, want %q", label, launchdServiceLabel)
	}

	label, err = launchdLabelFromServiceUnit("sentinel.custom")
	if err != nil {
		t.Fatalf("launchdLabelFromServiceUnit(\"sentinel.custom\") error: %v", err)
	}
	if label != "sentinel.custom" {
		t.Fatalf("label = %q, want sentinel.custom", label)
	}

	if _, err := launchdLabelFromServiceUnit("bad label"); err == nil {
		t.Fatal("expected error for whitespace in label")
	}
}

func TestParseLaunchdLastRun(t *testing.T) {
	t.Parallel()

	raw := `
service = {
	state = waiting
	last exit code = 0
}`
	if got := parseLaunchdLastRun(raw); got != "0" {
		t.Fatalf("parseLaunchdLastRun() = %q, want 0", got)
	}
}

func TestNormalizeLaunchdScope(t *testing.T) {
	t.Parallel()

	got, err := normalizeLaunchdScope(managerScopeUser)
	if err != nil || got != managerScopeUser {
		t.Fatalf("normalizeLaunchdScope(user) = %q, %v", got, err)
	}

	got, err = normalizeLaunchdScope(managerScopeSystem)
	if err != nil || got != managerScopeSystem {
		t.Fatalf("normalizeLaunchdScope(system) = %q, %v", got, err)
	}

	if _, err := normalizeLaunchdScope("invalid"); err == nil {
		t.Fatal("expected error for invalid scope")
	}

	got, err = normalizeLaunchdScope("")
	if err != nil {
		t.Fatalf("normalizeLaunchdScope(\"\") error: %v", err)
	}
	want := managerScopeUser
	if os.Geteuid() == 0 {
		want = managerScopeSystem
	}
	if got != want {
		t.Fatalf("normalizeLaunchdScope(\"\") = %q, want %q", got, want)
	}
}

func TestLaunchdPathsForSystemScope(t *testing.T) {
	t.Parallel()

	servicePath, err := userServicePathLaunchdForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("userServicePathLaunchdForScope(system) error: %v", err)
	}
	if servicePath != launchdSystemServicePath {
		t.Fatalf("service path = %q, want %q", servicePath, launchdSystemServicePath)
	}

	updaterPath, err := userAutoUpdatePathLaunchdForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("userAutoUpdatePathLaunchdForScope(system) error: %v", err)
	}
	if updaterPath != launchdSystemUpdaterPath {
		t.Fatalf("updater path = %q, want %q", updaterPath, launchdSystemUpdaterPath)
	}
}

func TestLaunchdDomainTarget(t *testing.T) {
	t.Parallel()

	if got := launchdDomainTarget(managerScopeSystem); got != managerScopeSystem {
		t.Fatalf("launchdDomainTarget(system) = %q, want %q", got, managerScopeSystem)
	}
	if got := launchdDomainTarget(managerScopeUser); !strings.HasPrefix(got, "gui/") {
		t.Fatalf("launchdDomainTarget(user) = %q, want gui/<uid>", got)
	}
}

func TestXMLEscape(t *testing.T) {
	t.Parallel()

	raw := `a&b<c>"'`
	got := xmlEscape(raw)
	want := "a&amp;b&lt;c&gt;&quot;&apos;"
	if got != want {
		t.Fatalf("xmlEscape(%q) = %q, want %q", raw, got, want)
	}
}

func TestXMLEscapeNoOp(t *testing.T) {
	t.Parallel()

	raw := "/usr/local/bin/sentinel"
	got := xmlEscape(raw)
	if got != raw {
		t.Fatalf("xmlEscape(%q) = %q, want unchanged", raw, got)
	}
}

func TestLaunchdUnitFileMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope string
		want  os.FileMode
	}{
		{managerScopeSystem, 0o644},
		{managerScopeUser, 0o600},
		{"anything-else", 0o600},
	}

	for _, tc := range tests {
		t.Run(tc.scope, func(t *testing.T) {
			t.Parallel()
			got := launchdUnitFileMode(tc.scope)
			if got != tc.want {
				t.Fatalf("launchdUnitFileMode(%q) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}

func TestLaunchdJobTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope string
		label string
		want  string
	}{
		{
			scope: managerScopeSystem,
			label: launchdServiceLabel,
			want:  "system/" + launchdServiceLabel,
		},
		{
			scope: managerScopeUser,
			label: launchdAutoUpdateLabel,
			want:  fmt.Sprintf("gui/%d/%s", os.Getuid(), launchdAutoUpdateLabel),
		},
	}

	for _, tc := range tests {
		t.Run(tc.scope+"/"+tc.label, func(t *testing.T) {
			t.Parallel()
			got := launchdJobTarget(tc.scope, tc.label)
			if got != tc.want {
				t.Fatalf("launchdJobTarget(%q, %q) = %q, want %q", tc.scope, tc.label, got, tc.want)
			}
		})
	}
}

func TestParseLaunchdLastRunEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no last exit line",
			raw:  "state = running\npid = 1234\n",
			want: "-",
		},
		{
			name: "empty value after equals",
			raw:  "last exit code = \n",
			want: "-",
		},
		{
			name: "non-zero exit code",
			raw:  "last exit code = 1\n",
			want: "1",
		},
		{
			name: "empty input",
			raw:  "",
			want: "-",
		},
		{
			name: "last exit mixed case",
			raw:  "  Last Exit Code = 42\n",
			want: "42",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseLaunchdLastRun(tc.raw)
			if got != tc.want {
				t.Fatalf("parseLaunchdLastRun() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnsureLaunchdScopePrivileges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   string
		wantErr bool
	}{
		{
			name:    "user scope always allowed",
			scope:   managerScopeUser,
			wantErr: false,
		},
		{
			name:    "system scope requires root",
			scope:   managerScopeSystem,
			wantErr: os.Geteuid() != 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ensureLaunchdScopePrivileges(tc.scope)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEnsureLaunchdSupportedOnLinux(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies non-darwin behavior")
	}
	err := ensureLaunchdSupported()
	if err == nil {
		t.Fatal("expected error on non-darwin OS")
	}
	if !strings.Contains(err.Error(), "macOS only") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNormalizeLaunchdScopeLaunchdAlias(t *testing.T) {
	t.Parallel()

	got, err := normalizeLaunchdScope(managerScopeLaunchd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := managerScopeUser
	if os.Geteuid() == 0 {
		want = managerScopeSystem
	}
	if got != want {
		t.Fatalf("normalizeLaunchdScope(launchd) = %q, want %q", got, want)
	}
}

func TestNormalizeLaunchdScopeCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"User", managerScopeUser},
		{"SYSTEM", managerScopeSystem},
		{" user ", managerScopeUser},
		{"Launchd", managerScopeUser}, // non-root
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeLaunchdScope(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := tc.want
			// For auto-like scopes, root changes the result
			if (strings.TrimSpace(strings.ToLower(tc.input)) == "launchd") && os.Geteuid() == 0 {
				want = managerScopeSystem
			}
			if got != want {
				t.Fatalf("normalizeLaunchdScope(%q) = %q, want %q", tc.input, got, want)
			}
		})
	}
}

func TestUserServicePathLaunchdForScopeUser(t *testing.T) {
	t.Parallel()

	path, err := userServicePathLaunchdForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	want := filepath.Join(home, "Library", "LaunchAgents", launchdServicePlistName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestUserAutoUpdatePathLaunchdForScopeUser(t *testing.T) {
	t.Parallel()

	path, err := userAutoUpdatePathLaunchdForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	want := filepath.Join(home, "Library", "LaunchAgents", launchdUpdaterPlistName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestUserServicePathLaunchdForScopeInvalid(t *testing.T) {
	t.Parallel()

	_, err := userServicePathLaunchdForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestUserAutoUpdatePathLaunchdForScopeInvalid(t *testing.T) {
	t.Parallel()

	_, err := userAutoUpdatePathLaunchdForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestLaunchdLabelFromServiceUnitSentinel(t *testing.T) {
	t.Parallel()

	label, err := launchdLabelFromServiceUnit("sentinel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if label != launchdServiceLabel {
		t.Fatalf("label = %q, want %q", label, launchdServiceLabel)
	}
}

func TestLaunchdLabelFromServiceUnitWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"tab in name", "bad\tname", true},
		{"newline in name", "bad\nname", true},
		{"cr in name", "bad\rname", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := launchdLabelFromServiceUnit(tc.input)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLaunchdStartIntervalDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  int
	}{
		{"1h", 3600},
		{"2h30m", 9000},
		{"10s", 10},
		{"1h30m", 5400},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := launchdStartInterval(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("launchdStartInterval(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestLaunchdStartIntervalZeroDuration(t *testing.T) {
	t.Parallel()

	_, err := launchdStartInterval("0s")
	if err == nil {
		t.Fatal("expected error for zero-second duration")
	}
}

func TestLaunchdStartIntervalNegative(t *testing.T) {
	t.Parallel()

	_, err := launchdStartInterval("-1")
	if err == nil {
		t.Fatal("expected error for negative value")
	}
}

func TestRenderLaunchdUserServicePlistXMLEscaping(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserServicePlist("/path/with <special>&chars", "/tmp/out.log", "/tmp/err.log")
	if strings.Contains(plist, "<special>") {
		t.Fatal("plist should escape angle brackets in exec path")
	}
	if !strings.Contains(plist, "&lt;special&gt;") {
		t.Fatal("plist should contain XML-escaped path")
	}
	if !strings.Contains(plist, "&amp;chars") {
		t.Fatal("plist should escape ampersand")
	}
}

func TestRenderLaunchdUserAutoUpdatePlistCustomInterval(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserAutoUpdatePlist(
		"/usr/bin/sentinel",
		"custom.label",
		managerScopeSystem,
		3600,
		"/var/log/out.log",
		"/var/log/err.log",
	)
	if !strings.Contains(plist, "<integer>3600</integer>") {
		t.Fatalf("plist missing custom interval: %s", plist)
	}
	if !strings.Contains(plist, "<string>-service=custom.label</string>") {
		t.Fatalf("plist missing custom service label: %s", plist)
	}
	if !strings.Contains(plist, "<string>-systemd-scope=system</string>") {
		t.Fatalf("plist missing system scope: %s", plist)
	}
}

func TestLaunchdLogPathsForScope(t *testing.T) {
	t.Parallel()

	// Only test user scope since it creates dirs in a user-writable location
	if runtime.GOOS != systemdSupportedOS && runtime.GOOS != launchdSupportedOS {
		t.Skip("test requires Linux or macOS")
	}

	stdout, stderr, err := launchdLogPathsForScope("test-svc", managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	wantDir := filepath.Join(home, ".sentinel", "logs")
	if !strings.HasPrefix(stdout, wantDir) {
		t.Fatalf("stdout path %q should be under %q", stdout, wantDir)
	}
	if !strings.HasPrefix(stderr, wantDir) {
		t.Fatalf("stderr path %q should be under %q", stderr, wantDir)
	}
	if !strings.Contains(stdout, "test-svc.out.log") {
		t.Fatalf("stdout should contain base name: %q", stdout)
	}
	if !strings.Contains(stderr, "test-svc.err.log") {
		t.Fatalf("stderr should contain base name: %q", stderr)
	}
}

func TestLaunchdLogPathsForScopeInvalid(t *testing.T) {
	t.Parallel()

	_, _, err := launchdLogPathsForScope("test-svc", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestUserServicePathLaunchdWrapper(t *testing.T) {
	t.Parallel()

	path, err := userServicePathLaunchd()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestUserStatusLaunchdWrapper(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != launchdSupportedOS {
		// On non-darwin, this should return an error from ensureLaunchdSupported
		// but the routing in UserStatus will not call this function on Linux.
		// Test the launchd internal directly.
		st, err := userStatusLaunchdForScope(managerScopeUser)
		if runtime.GOOS != launchdSupportedOS {
			// On Linux, launchctl is not available so it returns partial status
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if st.SystemctlAvailable {
				t.Fatal("launchctl should not be available on Linux")
			}
		}
		return
	}
}

func TestUserAutoUpdateStatusLaunchdForScope(t *testing.T) {
	t.Parallel()

	st, err := userAutoUpdateStatusLaunchdForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On non-darwin, launchctl won't be found
	if runtime.GOOS != launchdSupportedOS && st.SystemctlAvailable {
		t.Fatal("launchctl should not be available on Linux")
	}
	if st.ServicePath == "" {
		t.Fatal("expected non-empty service path")
	}
}

func TestUserAutoUpdateStatusLaunchdForScopeInvalid(t *testing.T) {
	t.Parallel()

	_, err := userAutoUpdateStatusLaunchdForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestReadLaunchdJobStateNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	loaded, active, lastRun := readLaunchdJobState(managerScopeUser, launchdServiceLabel)
	if loaded {
		t.Fatal("expected not loaded without launchctl")
	}
	if active != launchdStateInactive {
		t.Fatalf("active = %q, want %q", active, launchdStateInactive)
	}
	if lastRun != "-" {
		t.Fatalf("lastRun = %q, want -", lastRun)
	}
}

func TestRunLaunchctlNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	err := runLaunchctl("list")
	if err == nil {
		t.Fatal("expected error without launchctl")
	}
}

func TestRunLaunchctlOutputNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	out, err := runLaunchctlOutput("list")
	if err == nil {
		t.Fatal("expected error without launchctl")
	}
	if out != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}

func TestLaunchdBootstrapNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	err := launchdBootstrap(managerScopeUser, "/tmp/test.plist", "test.label")
	if err == nil {
		t.Fatal("expected error without launchctl")
	}
}

func TestLaunchdBootoutNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	// bootout on a system without launchctl: runLaunchctl fails,
	// then readLaunchdJobState also fails → not loaded → returns nil (job not loaded = success)
	err := launchdBootout(managerScopeUser, "test.label")
	if err != nil {
		t.Fatalf("expected nil (job not loaded), got: %v", err)
	}
}

func TestLaunchdKickstartNoLaunchctl(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies behavior without launchctl")
	}

	err := launchdKickstart(managerScopeUser, "test.label")
	if err == nil {
		t.Fatal("expected error without launchctl")
	}
}

func TestUserStatusLaunchdDirect(t *testing.T) {
	t.Parallel()

	st, err := userStatusLaunchd()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.ServicePath == "" {
		t.Fatal("expected non-empty ServicePath")
	}
}

func TestResolveLaunchdAutoUpdateInstallConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		opts              InstallUserAutoUpdateOptions
		wantErr           string
		checkInterval     int
		checkServiceLabel string
	}{
		{
			name: "defaults",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
			},
			checkInterval:     86400, // daily
			checkServiceLabel: launchdServiceLabel,
		},
		{
			name: "custom label and hourly",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
				ServiceUnit:  "custom.unit",
				OnCalendar:   "hourly",
			},
			checkInterval:     3600,
			checkServiceLabel: "custom.unit",
		},
		{
			name: "invalid scope",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "bogus",
			},
			wantErr: "invalid launchd scope",
		},
		{
			name: "invalid exec path",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel\nevil",
				SystemdScope: "user",
			},
			wantErr: "invalid executable path",
		},
		{
			name: "invalid service unit",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
				ServiceUnit:  "bad name",
			},
			wantErr: "invalid service unit name",
		},
		{
			name: "invalid on-calendar",
			opts: InstallUserAutoUpdateOptions{
				ExecPath:     "/usr/bin/sentinel",
				SystemdScope: "user",
				OnCalendar:   "never",
			},
			wantErr: "invalid on-calendar value for launchd",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := resolveLaunchdAutoUpdateInstallConfig(tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkInterval != 0 && cfg.interval != tc.checkInterval {
				t.Fatalf("interval = %d, want %d", cfg.interval, tc.checkInterval)
			}
			if tc.checkServiceLabel != "" && cfg.serviceLabel != tc.checkServiceLabel {
				t.Fatalf("serviceLabel = %q, want %q", cfg.serviceLabel, tc.checkServiceLabel)
			}
		})
	}
}

func TestWriteLaunchdAutoUpdatePlist(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "test.plist")

	cfg := launchdAutoUpdateInstallConfig{
		scope:        managerScopeUser,
		execPath:     "/usr/bin/sentinel",
		serviceLabel: launchdServiceLabel,
		interval:     86400,
		updaterPath:  plistPath,
		stdoutPath:   "/tmp/out.log",
		stderrPath:   "/tmp/err.log",
	}

	err := writeLaunchdAutoUpdatePlist(cfg)
	if err != nil {
		t.Fatalf("writeLaunchdAutoUpdatePlist() error: %v", err)
	}

	data, err := os.ReadFile(plistPath) //nolint:gosec // test reads from t.TempDir()
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "<integer>86400</integer>") {
		t.Fatal("plist missing interval")
	}
	if !strings.Contains(content, launchdAutoUpdateLabel) {
		t.Fatal("plist missing auto-update label")
	}

	// Verify file mode is user-only (0o600 for non-system)
	info, err := os.Stat(plistPath)
	if err != nil {
		t.Fatalf("stat plist: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWriteLaunchdAutoUpdatePlistSystemScope(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "test.plist")

	cfg := launchdAutoUpdateInstallConfig{
		scope:        managerScopeSystem,
		execPath:     "/usr/bin/sentinel",
		serviceLabel: launchdServiceLabel,
		interval:     3600,
		updaterPath:  plistPath,
		stdoutPath:   "/var/log/out.log",
		stderrPath:   "/var/log/err.log",
	}

	err := writeLaunchdAutoUpdatePlist(cfg)
	if err != nil {
		t.Fatalf("writeLaunchdAutoUpdatePlist() error: %v", err)
	}

	info, err := os.Stat(plistPath)
	if err != nil {
		t.Fatalf("stat plist: %v", err)
	}
	// System scope should be 0644
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestWriteLaunchdAutoUpdatePlistBadPath(t *testing.T) {
	t.Parallel()

	cfg := launchdAutoUpdateInstallConfig{
		scope:        managerScopeUser,
		execPath:     "/usr/bin/sentinel",
		serviceLabel: launchdServiceLabel,
		interval:     86400,
		updaterPath:  "/nonexistent/dir/test.plist",
		stdoutPath:   "/tmp/out.log",
		stderrPath:   "/tmp/err.log",
	}

	err := writeLaunchdAutoUpdatePlist(cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "write launchd autoupdate plist") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUserStatusLaunchdForScopeFileExists(t *testing.T) {
	t.Parallel()

	// On non-darwin, userStatusLaunchdForScope returns partial status without
	// launchctl checks. We can verify it checks the file system for the plist.
	tmpDir := t.TempDir()

	// Create a fake plist file at the expected path
	plistPath := filepath.Join(tmpDir, launchdServicePlistName)
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0o600); err != nil {
		t.Fatalf("write fake plist: %v", err)
	}

	// We can't easily override the path, but we can test with user scope
	// and verify the function returns successfully.
	st, err := userStatusLaunchdForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On non-darwin, launchctl is not available
	if runtime.GOOS != launchdSupportedOS {
		if st.SystemctlAvailable {
			t.Fatal("launchctl should not be available on non-darwin")
		}
	}
}

func TestUserStatusLaunchdForScopeInvalidScope(t *testing.T) {
	t.Parallel()

	_, err := userStatusLaunchdForScope("bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestUserAutoUpdateStatusLaunchdForScopeFileExists(t *testing.T) {
	t.Parallel()

	// Test that the function works on user scope and returns proper structure
	st, err := userAutoUpdateStatusLaunchdForScope(managerScopeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On non-darwin, launchctl won't be found
	if runtime.GOOS != launchdSupportedOS {
		if st.SystemctlAvailable {
			t.Fatal("launchctl should not be available on non-darwin")
		}
		// ServicePath and TimerPath should point to same plist (launchd has no separate timer)
		if st.ServicePath != st.TimerPath {
			t.Fatalf("ServicePath (%q) and TimerPath (%q) should match for launchd", st.ServicePath, st.TimerPath)
		}
	}
}

func TestUserAutoUpdateStatusLaunchdForScopeSystemOnNonDarwin(t *testing.T) {
	t.Parallel()

	st, err := userAutoUpdateStatusLaunchdForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.ServicePath != launchdSystemUpdaterPath {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, launchdSystemUpdaterPath)
	}
	if st.TimerPath != launchdSystemUpdaterPath {
		t.Fatalf("TimerPath = %q, want %q", st.TimerPath, launchdSystemUpdaterPath)
	}

	// Plist files in /Library won't exist on Linux
	if runtime.GOOS != launchdSupportedOS {
		if st.ServiceUnitExists {
			t.Fatal("service unit should not exist on non-darwin")
		}
		if st.TimerUnitExists {
			t.Fatal("timer unit should not exist on non-darwin")
		}
		if st.SystemctlAvailable {
			t.Fatal("launchctl should not be available on non-darwin")
		}
	}
}

func TestUserStatusLaunchdForScopeSystemOnNonDarwin(t *testing.T) {
	t.Parallel()

	st, err := userStatusLaunchdForScope(managerScopeSystem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.ServicePath != launchdSystemServicePath {
		t.Fatalf("ServicePath = %q, want %q", st.ServicePath, launchdSystemServicePath)
	}

	if runtime.GOOS != launchdSupportedOS {
		if st.UnitFileExists {
			t.Fatal("unit file should not exist on non-darwin")
		}
		if st.SystemctlAvailable {
			t.Fatal("launchctl should not be available on non-darwin")
		}
	}
}

func TestParseLaunchdLastRunNoEqualsSign(t *testing.T) {
	t.Parallel()

	// A line that matches "last exit" but has no '=' sign
	raw := "last exit status unknown\n"
	got := parseLaunchdLastRun(raw)
	if got != "-" {
		t.Fatalf("parseLaunchdLastRun() = %q, want -", got)
	}
}

func TestParseLaunchdLastRunMultipleMatches(t *testing.T) {
	t.Parallel()

	// Only the first matching line should be returned
	raw := `last exit code = 42
last exit code = 99
`
	got := parseLaunchdLastRun(raw)
	if got != "42" {
		t.Fatalf("parseLaunchdLastRun() = %q, want 42", got)
	}
}

func TestLaunchdLogPathsForScopeSystem(t *testing.T) {
	t.Parallel()

	if os.Geteuid() != 0 {
		// System log dir (/var/log/sentinel) may not be writable, but we can test
		// that it attempts the right paths. The function might fail on MkdirAll.
		_, _, err := launchdLogPathsForScope("sentinel", managerScopeSystem)
		// If it fails, it should be a permission error on /var/log/sentinel
		if err != nil {
			if !strings.Contains(err.Error(), "create sentinel log directory") {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		return
	}
}

func TestEnsureLaunchdScopePrivilegesUser(t *testing.T) {
	t.Parallel()

	// User scope should always be allowed regardless of euid
	err := ensureLaunchdScopePrivileges(managerScopeUser)
	if err != nil {
		t.Fatalf("user scope should always be allowed: %v", err)
	}
}

func TestEnsureLaunchdScopePrivilegesEmptyScope(t *testing.T) {
	t.Parallel()

	// Empty string scope should be allowed (it's not "system")
	err := ensureLaunchdScopePrivileges("")
	if err != nil {
		t.Fatalf("empty scope should be allowed: %v", err)
	}
}

func TestResolveLaunchdAutoUpdateInstallConfigSystemScopeNonRoot(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("test requires non-root")
	}

	_, err := resolveLaunchdAutoUpdateInstallConfig(InstallUserAutoUpdateOptions{
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

func TestResolveLaunchdAutoUpdateInstallConfigZeroDuration(t *testing.T) {
	t.Parallel()

	_, err := resolveLaunchdAutoUpdateInstallConfig(InstallUserAutoUpdateOptions{
		ExecPath:     "/usr/bin/sentinel",
		SystemdScope: "user",
		OnCalendar:   "0s",
	})
	if err == nil {
		t.Fatal("expected error for zero-second duration")
	}
}

func TestLaunchdStartIntervalCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  int
	}{
		{"Daily", 86400},
		{"HOURLY", 3600},
		{"Weekly", 604800},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := launchdStartInterval(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLaunchdStartIntervalNegativeDuration(t *testing.T) {
	t.Parallel()

	_, err := launchdStartInterval("-5m")
	if err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestLaunchdLabelFromServiceUnitWhitespaceOnly(t *testing.T) {
	t.Parallel()

	label, err := launchdLabelFromServiceUnit("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if label != launchdServiceLabel {
		t.Fatalf("label = %q, want %q", label, launchdServiceLabel)
	}
}

func TestRenderLaunchdUserServicePlistLogPaths(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserServicePlist("/usr/bin/sentinel", "/var/log/out.log", "/var/log/err.log")
	if !strings.Contains(plist, "<string>/var/log/out.log</string>") {
		t.Fatal("plist missing stdout path")
	}
	if !strings.Contains(plist, "<string>/var/log/err.log</string>") {
		t.Fatal("plist missing stderr path")
	}
	if !strings.Contains(plist, "<key>RunAtLoad</key>") {
		t.Fatal("plist missing RunAtLoad key")
	}
	if !strings.Contains(plist, "<key>KeepAlive</key>") {
		t.Fatal("plist missing KeepAlive key")
	}
}

func TestRenderLaunchdUserAutoUpdatePlistLabel(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserAutoUpdatePlist(
		"/usr/bin/sentinel",
		launchdServiceLabel,
		managerScopeUser,
		86400,
		"/tmp/out.log",
		"/tmp/err.log",
	)
	if !strings.Contains(plist, "<string>"+launchdAutoUpdateLabel+"</string>") {
		t.Fatal("plist missing auto-update label")
	}
}

func TestNormalizeLaunchdScopeAutoAlias(t *testing.T) {
	t.Parallel()

	got, err := normalizeLaunchdScope("auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := managerScopeUser
	if os.Geteuid() == 0 {
		want = managerScopeSystem
	}
	if got != want {
		t.Fatalf("normalizeLaunchdScope(auto) = %q, want %q", got, want)
	}
}

func TestXMLEscapeEmptyString(t *testing.T) {
	t.Parallel()

	got := xmlEscape("")
	if got != "" {
		t.Fatalf("xmlEscape(\"\") = %q, want empty", got)
	}
}

func TestLaunchdDomainTargetUser(t *testing.T) {
	t.Parallel()

	got := launchdDomainTarget(managerScopeUser)
	expected := fmt.Sprintf("gui/%d", os.Getuid())
	if got != expected {
		t.Fatalf("launchdDomainTarget(user) = %q, want %q", got, expected)
	}
}

func TestReadLaunchdJobStateOnNonDarwin(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == launchdSupportedOS {
		t.Skip("test verifies non-darwin behavior")
	}

	// For any label, readLaunchdJobState should return not-loaded on non-darwin
	loaded, active, lastRun := readLaunchdJobState(managerScopeSystem, launchdAutoUpdateLabel)
	if loaded {
		t.Fatal("should not be loaded on non-darwin")
	}
	if active != launchdStateInactive {
		t.Fatalf("active = %q, want %q", active, launchdStateInactive)
	}
	if lastRun != "-" {
		t.Fatalf("lastRun = %q, want -", lastRun)
	}
}
