package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAlertWebhookPayloadSerialization(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	payload := AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     map[string]any{"id": 42, "title": "CPU high"},
		Host:      "web-01",
		Timestamp: ts,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["event"] != "alert.created" {
		t.Errorf("event = %v, want alert.created", decoded["event"])
	}
	if decoded["host"] != "web-01" {
		t.Errorf("host = %v, want web-01", decoded["host"])
	}
	alert, ok := decoded["alert"].(map[string]any)
	if !ok {
		t.Fatalf("alert is not a map: %T", decoded["alert"])
	}
	if alert["title"] != "CPU high" {
		t.Errorf("alert.title = %v, want CPU high", alert["title"])
	}
}

func TestNilNotifierIsNoOp(t *testing.T) {
	t.Parallel()

	var n *Notifier
	err := n.Send(context.Background(), AlertWebhookPayload{
		Event: "alert.created",
	})
	if err != nil {
		t.Errorf("nil notifier Send returned error: %v", err)
	}

	// SendAsync on nil should not panic.
	n.SendAsync(AlertWebhookPayload{Event: "alert.created"})
}

func TestNewWithEmptyURLReturnsNil(t *testing.T) {
	t.Parallel()

	n := New("", nil)
	if n != nil {
		t.Errorf("New(\"\") = %v, want nil", n)
	}
}

func TestDefaultEvents(t *testing.T) {
	t.Parallel()

	n := New("http://example.com/hook", nil)
	if n == nil {
		t.Fatal("New returned nil for non-empty URL")
		return
	}
	if !n.events["alert.created"] {
		t.Error("default events should include alert.created")
	}
	if !n.events["alert.resolved"] {
		t.Error("default events should include alert.resolved")
	}
	if n.events["alert.acked"] {
		t.Error("default events should not include alert.acked")
	}
}

func TestEventFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured []string
		event      string
		wantSend   bool
	}{
		{
			name:       "configured event is sent",
			configured: []string{"alert.created"},
			event:      "alert.created",
			wantSend:   true,
		},
		{
			name:       "unconfigured event is skipped",
			configured: []string{"alert.created"},
			event:      "alert.resolved",
			wantSend:   false,
		},
		{
			name:       "acked when configured",
			configured: []string{"alert.acked"},
			event:      "alert.acked",
			wantSend:   true,
		},
		{
			name:       "all three configured",
			configured: []string{"alert.created", "alert.resolved", "alert.acked"},
			event:      "alert.acked",
			wantSend:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var received atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Store(true)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			n := New(srv.URL, tc.configured)
			err := n.Send(context.Background(), AlertWebhookPayload{
				Event:     tc.event,
				Alert:     map[string]string{"title": "test"},
				Host:      "host",
				Timestamp: time.Now(),
			})
			if err != nil {
				t.Fatalf("Send returned error: %v", err)
			}

			if received.Load() != tc.wantSend {
				t.Errorf("received = %v, want %v", received.Load(), tc.wantSend)
			}
		})
	}
}

func TestSendDeliversPayload(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var receivedBody []byte
	var receivedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedMethod = r.Method
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.created"})
	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	err := n.Send(context.Background(), AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     map[string]string{"title": "disk full"},
		Host:      "db-01",
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", receivedMethod)
	}

	var decoded AlertWebhookPayload
	if err := json.Unmarshal(receivedBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if decoded.Event != "alert.created" {
		t.Errorf("event = %s, want alert.created", decoded.Event)
	}
	if decoded.Host != "db-01" {
		t.Errorf("host = %s, want db-01", decoded.Host)
	}
}

func TestSendReturnsErrorOnServerError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.created"})
	err := n.Send(context.Background(), AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     map[string]string{"title": "test"},
		Host:      "host",
		Timestamp: time.Now(),
	})

	// After all retries the Send must return an error for 5xx responses.
	if err == nil {
		t.Error("expected error for 500 responses")
	}

	// At least one attempt must have been made.
	if got := attempts.Load(); got < 1 {
		t.Errorf("attempts = %d, want >= 1", got)
	}
}

func TestSendReturnsErrorOnClientError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.resolved"})
	err := n.Send(context.Background(), AlertWebhookPayload{
		Event:     "alert.resolved",
		Alert:     map[string]string{"title": "test"},
		Host:      "host",
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

func TestSendContextTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server.
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.created"})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := n.Send(ctx, AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     map[string]string{"title": "test"},
		Host:      "host",
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error on context timeout")
	}
}

func TestSendAsyncDelivers(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.created"})
	n.SendAsync(AlertWebhookPayload{
		Event:     "alert.created",
		Alert:     map[string]string{"title": "test"},
		Host:      "host",
		Timestamp: time.Now(),
	})

	// Wait for the async goroutine to complete.
	deadline := time.After(5 * time.Second)
	for !received.Load() {
		select {
		case <-deadline:
			t.Fatal("SendAsync did not deliver within timeout")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestSendAsyncSkipsFilteredEvent(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, []string{"alert.created"})
	n.SendAsync(AlertWebhookPayload{
		Event:     "alert.acked",
		Alert:     map[string]string{"title": "test"},
		Host:      "host",
		Timestamp: time.Now(),
	})

	// Give it a moment — it should NOT fire.
	time.Sleep(200 * time.Millisecond)
	if received.Load() {
		t.Error("SendAsync should not have delivered a filtered event")
	}
}
