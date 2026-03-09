package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	fastshot "github.com/opus-domini/fast-shot"
)

// AlertWebhookPayload is the JSON body sent to the webhook endpoint.
type AlertWebhookPayload struct {
	Event     string    `json:"event"` // "alert.created", "alert.resolved", "alert.acked"
	Alert     any       `json:"alert"` // alert details
	Host      string    `json:"host"`  // hostname
	Timestamp time.Time `json:"timestamp"`
}

// Notifier sends HTTP webhook notifications for alert lifecycle events.
// A nil *Notifier is safe to call (all methods are no-ops).
type Notifier struct {
	url    string
	events map[string]bool
	client fastshot.ClientHttpMethods
}

// New creates a Notifier. If url is empty the notifier is disabled (Send is
// a no-op). When events is empty and url is non-empty, the default event
// set ("alert.created", "alert.resolved") is used.
func New(url string, events []string) *Notifier {
	if url == "" {
		return nil
	}
	eventSet := make(map[string]bool, len(events))
	if len(events) == 0 {
		eventSet["alert.created"] = true
		eventSet["alert.resolved"] = true
	} else {
		for _, e := range events {
			eventSet[e] = true
		}
	}
	client := fastshot.NewClient(url).
		Config().SetTimeout(10 * time.Second).
		Build()
	return &Notifier{
		url:    url,
		events: eventSet,
		client: client,
	}
}

// URL returns the configured webhook URL, or "" if the notifier is nil/disabled.
func (n *Notifier) URL() string {
	if n == nil {
		return ""
	}
	return n.url
}

// Events returns the list of enabled event names. Returns nil for a nil receiver.
func (n *Notifier) Events() []string {
	if n == nil {
		return nil
	}
	out := make([]string, 0, len(n.events))
	for e := range n.events {
		out = append(out, e)
	}
	return out
}

// Send delivers a webhook notification if the event is enabled.
// It is safe to call on a nil receiver.
func (n *Notifier) Send(ctx context.Context, payload AlertWebhookPayload) error {
	if n == nil || n.url == "" {
		return nil
	}
	if !n.events[payload.Event] {
		return nil
	}

	resp, err := n.client.POST("").
		Body().AsJSON(payload).
		Context().Set(ctx).
		Retry().SetExponentialBackoffWithJitter(1*time.Second, 3, 2.0).
		Retry().WithMaxDelay(5 * time.Second).
		Retry().WithRetryCondition(func(r *fastshot.Response) bool {
		return r.Status().Is5xxServerError()
	}).
		Send()
	if err != nil {
		return fmt.Errorf("alert webhook delivery failed: %w", err)
	}
	defer resp.Body().Close()
	if resp.Status().IsError() {
		return fmt.Errorf("alert webhook rejected: status %d", resp.Status().Code())
	}
	slog.Info("alert webhook delivered", "url", n.url, "event", payload.Event, "status", resp.Status().Code())
	return nil
}

// SendJSON delivers an arbitrary JSON payload to the webhook URL.
// It bypasses event filtering — the caller decides when to call it.
// Safe to call on a nil receiver.
func (n *Notifier) SendJSON(ctx context.Context, payload any) error {
	if n == nil || n.url == "" {
		return nil
	}

	resp, err := n.client.POST("").
		Body().AsJSON(payload).
		Context().Set(ctx).
		Retry().SetExponentialBackoffWithJitter(1*time.Second, 3, 2.0).
		Retry().WithMaxDelay(5 * time.Second).
		Retry().WithRetryCondition(func(r *fastshot.Response) bool {
		return r.Status().Is5xxServerError()
	}).
		Send()
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body().Close()
	if resp.Status().IsError() {
		return fmt.Errorf("webhook rejected: status %d", resp.Status().Code())
	}
	slog.Info("webhook delivered", "url", n.url, "status", resp.Status().Code())
	return nil
}

// SendAsync fires a webhook notification in a background goroutine.
// Errors are logged but not returned. Safe to call on a nil receiver.
func (n *Notifier) SendAsync(payload AlertWebhookPayload) {
	if n == nil || n.url == "" {
		return
	}
	if !n.events[payload.Event] {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := n.Send(ctx, payload); err != nil {
			slog.Warn("alert webhook async send failed", "event", payload.Event, "error", err)
		}
	}()
}
