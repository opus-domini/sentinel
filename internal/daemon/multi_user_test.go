package daemon

import (
	"strings"
	"testing"
)

func TestRenderUserUnitOmitsHardeningDirectives(t *testing.T) {
	t.Parallel()

	unit := renderUserUnit("/usr/bin/sentinel")
	if strings.Contains(unit, "NoNewPrivileges=") {
		t.Error("expected NoNewPrivileges= directive to be absent from user unit")
	}
	if strings.Contains(unit, "SystemCallArchitectures=") {
		t.Error("expected SystemCallArchitectures= directive to be absent from user unit")
	}
}

func TestRenderUserAutoUpdateUnitOmitsHardeningDirectives(t *testing.T) {
	t.Parallel()

	unit := renderUserAutoUpdateUnit("/usr/bin/sentinel", "sentinel", "user")
	if strings.Contains(unit, "NoNewPrivileges=") {
		t.Error("expected NoNewPrivileges= directive to be absent from auto-update unit")
	}
	if strings.Contains(unit, "SystemCallArchitectures=") {
		t.Error("expected SystemCallArchitectures= directive to be absent from auto-update unit")
	}
}
