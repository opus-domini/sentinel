package events

import (
	"testing"
	"time"
)

func TestPublishAssignsMonotonicEventID(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	ch, unsubscribe := hub.Subscribe(4)
	t.Cleanup(unsubscribe)

	hub.Publish(NewEvent(TypeTmuxSessions, map[string]any{"session": "dev"}))
	hub.Publish(NewEvent(TypeTmuxActivity, map[string]any{"session": "dev"}))

	first := <-ch
	second := <-ch

	if first.EventID <= 0 {
		t.Fatalf("first.EventID = %d, want > 0", first.EventID)
	}
	if second.EventID <= first.EventID {
		t.Fatalf("second.EventID = %d, want > %d", second.EventID, first.EventID)
	}
}

func TestPublishAssignsTimestampWhenMissing(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	ch, unsubscribe := hub.Subscribe(2)
	t.Cleanup(unsubscribe)

	hub.Publish(Event{Type: TypeReady})

	select {
	case evt := <-ch:
		if evt.Timestamp == "" {
			t.Fatalf("event timestamp should be set")
		}
		if _, err := time.Parse(time.RFC3339, evt.Timestamp); err != nil {
			t.Fatalf("timestamp parse error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive published event")
	}
}
