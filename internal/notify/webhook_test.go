package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
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

	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
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

func TestSendJSONDoesNotRetryClientErrors(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := New(srv.URL)
	err := n.SendJSON(context.Background(), map[string]any{"ok": true})
	if err == nil {
		t.Fatal("expected SendJSON error for 400 response")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (4xx must not be retried)", got)
	}
}

func TestSendJSONRetriesServerErrors(t *testing.T) {
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
		t.Fatal("expected error for 500 responses")
	}

	if got := attempts.Load(); got != maxAttempts {
		t.Errorf("attempts = %d, want %d", got, maxAttempts)
	}
}

func TestSendJSONStopsRetryingAfterSuccess(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL)
	if err := n.SendJSON(context.Background(), map[string]any{"ok": true}); err != nil {
		t.Fatalf("SendJSON returned error: %v", err)
	}

	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2 (retry then stop on success)", got)
	}
}

func TestSendJSONContextTimeout(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	// Defers run last-in-first-out: the handler is released before the server
	// is closed, otherwise srv.Close would block on it forever.
	defer srv.Close()
	defer close(release)

	n := New(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := n.SendJSON(ctx, map[string]any{"ok": true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendJSON error = %v, want context.DeadlineExceeded", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (an expired context must not be retried)", got)
	}
}

func TestSendJSONAbandonsRetryOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		cancel()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := New(srv.URL).SendJSON(ctx, map[string]any{"ok": true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendJSON error = %v, want context.Canceled", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (backoff must abort on cancel)", got)
	}
}

func TestSendJSONDoesNotExposeWebhookURLInErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	webhookURL := srv.URL + "/private-path?token=webhook-secret"
	srv.Close()

	err := New(webhookURL).SendJSON(context.Background(), map[string]any{"ok": true})
	if err == nil {
		t.Fatal("expected delivery error")
	}
	if strings.Contains(err.Error(), webhookURL) ||
		strings.Contains(err.Error(), "private-path") ||
		strings.Contains(err.Error(), "webhook-secret") {
		t.Fatalf("delivery error exposed webhook URL: %v", err)
	}
}
