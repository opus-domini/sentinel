// Package events publishes in-process Sentinel events.
package events

import (
	"sync"
	"time"
)

// PresenceExpiry is the TTL for client presence heartbeats.
const PresenceExpiry = 30 * time.Second

const (
	// TypeReady announces that the events stream is connected.
	TypeReady = "events.ready"
	// TypeTmuxSessions announces that tmux session projections changed.
	TypeTmuxSessions = "tmux.sessions.updated"
	// TypeTmuxInspector announces that tmux inspector projections changed.
	TypeTmuxInspector = "tmux.inspector.updated"
	// TypeTmuxActivity announces that tmux activity stats changed.
	TypeTmuxActivity = "tmux.activity.updated"
	// TypeOpsOverview announces that the ops overview changed.
	TypeOpsOverview = "ops.overview.updated"
	// TypeOpsServices announces that ops service state changed.
	TypeOpsServices = "ops.services.updated"
	// TypeOpsJob announces that an ops job changed.
	TypeOpsJob = "ops.job.updated"
	// TypeOpsMetrics announces that ops metrics changed.
	TypeOpsMetrics = "ops.metrics.updated"
	// TypeScheduleUpdated announces that scheduler state changed.
	TypeScheduleUpdated = "ops.schedule.updated"
)

// Event represents event data.
type Event struct {
	EventID   int64          `json:"eventId"`
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// NewEvent creates event.
func NewEvent(eventType string, payload map[string]any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
}

// Hub represents hub data.
type Hub struct {
	mu          sync.RWMutex
	nextSubID   int64
	nextEventID int64
	subscribers map[int64]chan Event
}

// NewHub creates hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[int64]chan Event),
	}
}

// Subscribe subscribes to value.
func (h *Hub) Subscribe(buffer int) (<-chan Event, func()) {
	if h == nil {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan Event, buffer)

	h.mu.Lock()
	h.nextSubID++
	id := h.nextSubID
	h.subscribers[id] = ch
	h.mu.Unlock()

	unsubscribe := func() {
		// Close while holding the lock so Publish (which delivers under the same
		// lock) can never observe a half-removed subscriber and send on a closed
		// channel.
		h.mu.Lock()
		if current, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(current)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

// Publish publishes value.
func (h *Hub) Publish(event Event) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if event.EventID <= 0 {
		h.nextEventID++
		event.EventID = h.nextEventID
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	// Deliver while still holding the lock that unsubscribe uses to close
	// channels. This makes send and close mutually exclusive, so a subscriber
	// channel can never be closed mid-send and the non-blocking send below
	// cannot panic with "send on closed channel". Sends stay non-blocking via
	// the default case, so holding the lock here is bounded.
	for _, sub := range h.subscribers {
		select {
		case sub <- event:
		default:
			// Skip when client is slow; next state event will arrive.
		}
	}
}
