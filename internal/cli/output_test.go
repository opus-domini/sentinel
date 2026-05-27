package cli

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestValueStyleLeavesUnknownValuesPlain(t *testing.T) {
	t.Parallel()

	value := "custom-value"
	if got := valueStyle(value); got != stylePlain {
		t.Fatalf("valueStyle(%q) = %v, want %v", value, got, stylePlain)
	}
}
