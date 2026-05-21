package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestServiceLifecycleCommands covers the start/stop/restart/enable/disable
// leaf commands of `sentinel service`.
func TestServiceLifecycleCommands(t *testing.T) {
	cases := []struct {
		action  string
		wantOut string
	}{
		{"start", "service started"},
		{"stop", "service stopped"},
		{"restart", "service restarted"},
		{"enable", "service enabled"},
		{"disable", "service disabled"},
	}

	origControl := controlServiceFn
	t.Cleanup(func() { controlServiceFn = origControl })

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			var gotAction string
			controlServiceFn = func(action string) error {
				gotAction = action
				return nil
			}

			var out, errOut bytes.Buffer
			code := Run([]string{"service", tc.action}, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if gotAction != tc.action {
				t.Fatalf("controlServiceFn action = %q, want %q", gotAction, tc.action)
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q: %s", tc.wantOut, out.String())
			}
		})
	}
}

func TestServiceLifecycleCommandFailure(t *testing.T) {
	origControl := controlServiceFn
	t.Cleanup(func() { controlServiceFn = origControl })
	controlServiceFn = func(string) error { return errors.New("systemctl failed") }

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "restart"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service restart failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

func TestServiceLifecycleRejectsArgs(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "start", "extra"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
