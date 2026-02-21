package store

import (
	"encoding/hex"
	"testing"
)

func TestRandomID_Length(t *testing.T) {
	t.Parallel()
	id := randomID()
	if len(id) != 32 {
		t.Fatalf("expected 32 hex chars, got %d: %q", len(id), id)
	}
}

func TestRandomID_ValidHex(t *testing.T) {
	t.Parallel()
	id := randomID()
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("not valid hex: %q: %v", id, err)
	}
}

func TestRandomID_Unique(t *testing.T) {
	t.Parallel()
	const n = 100
	seen := make(map[string]struct{}, n)
	for range n {
		id := randomID()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id: %q", id)
		}
		seen[id] = struct{}{}
	}
}
