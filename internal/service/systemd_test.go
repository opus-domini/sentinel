package service

import (
	"strings"
	"testing"
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
