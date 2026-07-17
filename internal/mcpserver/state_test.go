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
