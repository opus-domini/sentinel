package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/daemon"
)

func TestServiceStatusNotInstalled(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) { return nil, nil }

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "no Sentinel service is installed") {
		t.Fatalf("stdout missing not-installed message: %s", out.String())
	}
}

// TestServiceStatusMultiScope verifies the status command renders every scope
// the daemon reports a unit in — the fix for the false negative when a system
// service is queried from a user shell.
func TestServiceStatusMultiScope(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{
			{Scope: "user", UserServiceStatus: daemon.UserServiceStatus{
				ServicePath: "/home/u/.config/systemd/user/sentinel.service", UnitFileExists: true,
			}},
			{Scope: "system", UserServiceStatus: daemon.UserServiceStatus{
				ServicePath: "/etc/systemd/system/sentinel.service", UnitFileExists: true,
			}},
		}, nil
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"user unit file: /home/u/.config/systemd/user/sentinel.service",
		"system unit file: /etc/systemd/system/sentinel.service",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}
