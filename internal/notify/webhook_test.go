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

func TestNewWithEmptyURLReturnsNil(t *testing.T) {
	t.Parallel()

	n := New("")
	if n != nil {
		t.Errorf("New(\"\") = %v, want nil", n)
	}
}

func TestNotifierAccessors(t *testing.T) {
	t.Parallel()

	var disabled *Notifier
	if got := disabled.URL(); got != "" {
		t.Fatalf("nil URL() = %q, want empty", got)
	}

	n := New("http://example.com/hook")
	if got := n.URL(); got != "http://example.com/hook" {
		t.Fatalf("URL() = %q, want configured URL", got)
	}
}

func TestSendJSONDeliversPayload(t *testing.T) {
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

	n := New(srv.URL)
	if err := n.SendJSON(context.Background(), map[string]any{"ok": true}); err != nil {
		t.Fatalf("SendJSON returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", receivedMethod)
	}

	var decoded map[string]bool
	if err := json.Unmarshal(receivedBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if !decoded["ok"] {
		t.Fatalf("decoded body = %#v, want ok=true", decoded)
	}
}

func TestSendJSONNilNotifierIsNoOp(t *testing.T) {
	t.Parallel()

	var n *Notifier
	if err := n.SendJSON(context.Background(), map[string]any{"ok": true}); err != nil {
		t.Fatalf("nil SendJSON returned error: %v", err)
	}
}

func TestSendJSONReturnsErrorOnClientError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := New(srv.URL)
	if err := n.SendJSON(context.Background(), map[string]any{"ok": true}); err == nil {
		t.Fatal("expected SendJSON error for 400 response")
	}
}

func TestSendJSONReturnsErrorOnServerError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := New(srv.URL)
	err := n.SendJSON(context.Background(), map[string]any{"ok": true})

	if err == nil {
		t.Error("expected error for 500 responses")
	}

	if got := attempts.Load(); got < 1 {
		t.Errorf("attempts = %d, want >= 1", got)
	}
}

func TestSendJSONContextTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate a slow server.
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := n.SendJSON(ctx, map[string]any{"ok": true})
	if err == nil {
		t.Error("expected error on context timeout")
	}
}
