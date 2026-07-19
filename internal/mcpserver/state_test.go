package mcpserver

import (
	"errors"
	"testing"
)

func TestStateRequiresSharedServerToken(t *testing.T) {
	state := NewState(false, false)

	if err := state.SetEnabled(true); !errors.Is(err, ErrTokenRequired) {
		t.Fatalf("SetEnabled(true) error = %v, want ErrTokenRequired", err)
	}
	if state.Enabled() {
		t.Fatal("state enabled without a configured token")
	}
}

func TestStateChangesLiveAvailability(t *testing.T) {
	state := NewState(false, true)

	if err := state.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true) error = %v", err)
	}
	if !state.Enabled() {
		t.Fatal("state remained disabled")
	}
	if err := state.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false) error = %v", err)
	}
	if state.Enabled() {
		t.Fatal("state remained enabled")
	}
}

func TestStateReportsTokenConfiguration(t *testing.T) {
	if (*State)(nil).TokenConfigured() {
		t.Fatal("nil state reported a configured token")
	}
	if NewState(false, false).TokenConfigured() {
		t.Fatal("state reported a token that was not configured")
	}
	if !NewState(false, true).TokenConfigured() {
		t.Fatal("state did not report its configured token")
	}
}
