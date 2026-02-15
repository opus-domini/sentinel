package service

import (
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
		86400,
		"/tmp/sentinel-updater.out.log",
		"/tmp/sentinel-updater.err.log",
	)
	for _, fragment := range []string{
		"<string>update</string>",
		"<string>apply</string>",
		"<string>-restart=true</string>",
		"<string>-service=" + launchdServiceLabel + "</string>",
		"<string>-systemd-scope=launchd</string>",
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

func TestXMLEscape(t *testing.T) {
	t.Parallel()

	raw := `a&b<c>"'`
	got := xmlEscape(raw)
	want := "a&amp;b&lt;c&gt;&quot;&apos;"
	if got != want {
		t.Fatalf("xmlEscape(%q) = %q, want %q", raw, got, want)
	}
}
