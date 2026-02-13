package events

import (
	"sync"
	"time"
)

const (
	TypeReady            = "events.ready"
	TypeTmuxSessions     = "tmux.sessions.updated"
	TypeTmuxInspector    = "tmux.inspector.updated"
	TypeTmuxActivity     = "tmux.activity.updated"
	TypeRecoveryOverview = "recovery.overview.updated"
	TypeRecoveryJob      = "recovery.job.updated"
)

type Event struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func NewEvent(eventType string, payload map[string]any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
}

type Hub struct {
	mu          sync.RWMutex
	nextID      int64
	subscribers map[int64]chan Event
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[int64]chan Event),
	}
}

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
	h.nextID++
	id := h.nextID
	h.subscribers[id] = ch
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		current, ok := h.subscribers[id]
		if ok {
			delete(h.subscribers, id)
		}
		h.mu.Unlock()
		if ok {
			close(current)
		}
	}
	return ch, unsubscribe
}

func (h *Hub) Publish(event Event) {
	if h == nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subscribers {
		select {
		case sub <- event:
		default:
			// Skip when client is slow; next state event will arrive.
		}
	}
}
