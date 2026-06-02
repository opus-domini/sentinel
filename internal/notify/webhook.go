// Package notify delivers Sentinel notifications.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	fastshot "github.com/opus-domini/fast-shot"
)

// Notifier sends HTTP webhook notifications.
// A nil *Notifier is safe to call (all methods are no-ops).
type Notifier struct {
	url    string
	client fastshot.ClientHttpMethods
}

// New creates a Notifier. If url is empty the notifier is disabled.
func New(url string) *Notifier {
	if url == "" {
		return nil
	}
	client := fastshot.NewClient(url).
		Config().SetTimeout(10 * time.Second).
		Build()
	return &Notifier{
		url:    url,
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
