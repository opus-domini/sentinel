package events

import (
	"sync"
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

// TestHubConcurrentPublishUnsubscribe exercises the send/close window that used
// to panic with "send on closed channel": Publish delivers to subscriber
// channels while subscribers churn (subscribe → drain → unsubscribe). Run under
// -race, a regression makes the test binary panic/race instead of passing.
func TestHubConcurrentPublishUnsubscribe(t *testing.T) {
	t.Parallel()

	hub := NewHub()

	const (
		publishers  = 4
		subscribers = 16
		iterations  = 200
	)

	stop := make(chan struct{})
	var pubWG, subWG sync.WaitGroup

	for range publishers {
		pubWG.Add(1)
		go func() {
			defer pubWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
					hub.Publish(NewEvent(TypeOpsMetrics, nil))
				}
			}
		}()
	}

	for range subscribers {
		subWG.Add(1)
		go func() {
			defer subWG.Done()
			for range iterations {
				ch, unsubscribe := hub.Subscribe(1)
				drained := make(chan struct{})
				go func() {
					for range ch { //nolint:revive // drain until unsubscribe closes ch
					}
					close(drained)
				}()
				unsubscribe()
				<-drained
			}
		}()
	}

	subWG.Wait()
	close(stop)
	pubWG.Wait()

	// Hub must still deliver after the churn.
	ch, unsubscribe := hub.Subscribe(1)
	t.Cleanup(unsubscribe)
	hub.Publish(NewEvent(TypeReady, nil))
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("hub stopped delivering after concurrent churn")
	}
}
