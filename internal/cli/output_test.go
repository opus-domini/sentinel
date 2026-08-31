package cli

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ansiSequence matches the SGR escapes lipgloss emits, so the styled output
// can be asserted on its text content alone.
var ansiSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ttyWriter captures output while reporting the file descriptor of a real
// pseudo-terminal, which is what makes shouldUsePrettyOutput take its styled
// branch. Without it every CLI test writes to a *bytes.Buffer and only the
// plain branch is ever executed.
type ttyWriter struct {
	buf bytes.Buffer
	fd  uintptr
}

func (w *ttyWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }

func (w *ttyWriter) Fd() uintptr { return w.fd }

func (w *ttyWriter) text() string { return ansiSequence.ReplaceAllString(w.buf.String(), "") }

func newTTYWriter(t *testing.T) *ttyWriter {
	t.Helper()
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	t.Cleanup(func() { _ = pty.Close() })
	return &ttyWriter{fd: pty.Fd()}
}

// prettyEnv clears the guards that force plain output so the styled branch is
// reachable regardless of the environment the tests run in.
func prettyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
}

func TestPrintRowsPlainOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printRows(&buf, []outputRow{
		{Key: "service file", Value: "/tmp/sentinel.service"},
		{Key: "active", Value: "true"},
	})

	got := buf.String()
	for _, fragment := range []string{
		"service file: /tmp/sentinel.service",
		"active: true",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("plain output missing %q: %s", fragment, got)
		}
	}
}

func TestPrintRowsAlignsKeysOnTerminal(t *testing.T) {
	prettyEnv(t)
	w := newTTYWriter(t)

	printRows(w, []outputRow{
		{Key: "service file", Value: "/tmp/sentinel.service"},
		{Key: "active", Value: "true"},
	})

	want := "service file  /tmp/sentinel.service\nactive        true\n"
	if got := w.text(); got != want {
		t.Fatalf("aligned output = %q, want %q", got, want)
	}
}

func TestPrintersRenderTextOnTerminal(t *testing.T) {
	prettyEnv(t)

	cases := []struct {
		name  string
		print func(w *ttyWriter)
		want  string
	}{
		{"heading", func(w *ttyWriter) { printHeading(w, "doctor") }, "doctor\n"},
		{"notice", func(w *ttyWriter) { printNotice(w, "update applied") }, "update applied\n"},
		{"header", func(w *ttyWriter) { reportHeader(w, "config", "initialization") }, "config  initialization\n\n"},
		{"header without detail", func(w *ttyWriter) { reportHeader(w, "config", "") }, "config\n\n"},
		{"done", func(w *ttyWriter) { done(w, "wrote", "/tmp/config.toml") }, "wrote /tmp/config.toml\n"},
		{"empty", func(w *ttyWriter) { empty(w, "no sessions") }, "no sessions\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTTYWriter(t)
			tc.print(w)
			if got := w.text(); got != tc.want {
				t.Fatalf("%s output = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestShouldUsePrettyOutputGuards(t *testing.T) {
	prettyEnv(t)
	terminal := newTTYWriter(t)

	if !shouldUsePrettyOutput(terminal) {
		t.Fatal("shouldUsePrettyOutput(terminal) = false, want true")
	}
	if shouldUsePrettyOutput(&bytes.Buffer{}) {
		t.Fatal("shouldUsePrettyOutput(buffer) = true, want false")
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	if shouldUsePrettyOutput(&ttyWriter{fd: devNull.Fd()}) {
		t.Fatal("shouldUsePrettyOutput(non-terminal fd) = true, want false")
	}

	t.Setenv("TERM", "DUMB")
	if shouldUsePrettyOutput(terminal) {
		t.Fatal("shouldUsePrettyOutput() honoured a dumb terminal")
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	if shouldUsePrettyOutput(terminal) {
		t.Fatal("shouldUsePrettyOutput() ignored NO_COLOR")
	}
	if got := renderStyle(terminal, styleSuccess, "ok"); got != "ok" {
		t.Fatalf("renderStyle() under NO_COLOR = %q, want plain", got)
	}
}

func TestRenderStyleShortCircuits(t *testing.T) {
	prettyEnv(t)
	terminal := newTTYWriter(t)

	if got := renderStyle(terminal, stylePlain, "value"); got != "value" {
		t.Fatalf("renderStyle(stylePlain) = %q, want %q", got, "value")
	}
	if got := renderStyle(terminal, styleBold, ""); got != "" {
		t.Fatalf("renderStyle(empty) = %q, want empty", got)
	}
	for _, kind := range []textStyle{styleBold, styleMuted, styleSuccess, styleWarning, styleDanger} {
		got := renderStyle(terminal, kind, "value")
		if stripped := ansiSequence.ReplaceAllString(got, ""); stripped != "value" {
			t.Fatalf("renderStyle(%v) = %q, want %q after stripping escapes", kind, stripped, "value")
		}
	}
}

func TestValueStyleMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  textStyle
	}{
		{"true", styleSuccess},
		{"Active", styleSuccess},
		{"  enabled  ", styleSuccess},
		{"loaded", styleSuccess},
		{"ok", styleSuccess},
		{"yes", styleSuccess},
		{"up", styleSuccess},
		{"false", styleDanger},
		{"inactive", styleDanger},
		{"disabled", styleDanger},
		{"not-loaded", styleDanger},
		{"unavailable", styleDanger},
		{"not-found", styleDanger},
		{"failed", styleDanger},
		{"error", styleDanger},
		{"down", styleDanger},
		{"-", styleWarning},
		{"unknown", styleWarning},
		{"n/a", styleWarning},
		{"custom-value", stylePlain},
		{"", stylePlain},
	}
	for _, tc := range cases {
		if got := valueStyle(tc.value); got != tc.want {
			t.Fatalf("valueStyle(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
