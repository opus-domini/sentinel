package alerts

import (
	"errors"
	"testing"
)

func TestAlertStatusConstants(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"open":     StatusOpen,
		"acked":    StatusAcked,
		"resolved": StatusResolved,
	}
	for want, got := range tests {
		if got != want {
			t.Fatalf("status constant = %q, want %q", got, want)
		}
	}
}

func TestErrInvalidFilter(t *testing.T) {
	t.Parallel()

	if !errors.Is(ErrInvalidFilter, ErrInvalidFilter) {
		t.Fatal("ErrInvalidFilter should match itself")
	}
}
