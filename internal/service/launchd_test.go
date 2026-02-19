package service

import (
	"os"
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
		{input: "daily", want: 86400},
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
