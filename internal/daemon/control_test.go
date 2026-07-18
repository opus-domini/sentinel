package daemon

import (
	"strings"
	"testing"
)

func TestValidServiceAction(t *testing.T) {
	t.Parallel()

	for _, action := range ServiceActions {
		if !validServiceAction(action) {
			t.Errorf("validServiceAction(%q) = false, want true", action)
		}
	}
	for _, action := range []string{"", "bogus", "START", "kill", "reload"} {
		if validServiceAction(action) {
			t.Errorf("validServiceAction(%q) = true, want false", action)
		}
	}
}

func TestControlRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	err := Control("bogus", ScopeAuto)
	if err == nil {
		t.Fatal("Control(\"bogus\") error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "unknown service action") {
		t.Fatalf("error = %v, want an unknown-service-action error", err)
	}
}
