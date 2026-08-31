// Package notify delivers Sentinel notifications.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	fastshot "github.com/opus-domini/fast-shot"
)

// maxAttempts bounds how many times SendJSON POSTs a single payload before
// giving up.
const maxAttempts = 3

// retryInterval is the backoff before the first retry; it doubles per attempt.
// A var so tests can collapse the backoff.
var retryInterval = 1 * time.Second

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
// Transport failures and 5xx responses are retried with exponential backoff;
// ctx cancellation aborts the backoff instead of outliving it.
// Safe to call on a nil receiver.
func (n *Notifier) SendJSON(ctx context.Context, payload any) error {
	if n == nil || n.url == "" {
		return nil
	}

	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			if err := wait(ctx, retryInterval<<(attempt-1)); err != nil {
				return err
			}
		}
		retry, err := n.post(ctx, payload)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}
	return lastErr
}

// post performs a single delivery attempt and reports whether the failure is
// worth retrying. The transport error is deliberately discarded: it wraps a
// *url.Error whose message embeds the full webhook URL, which is itself the
// credential for common targets such as Slack and Discord.
func (n *Notifier) post(ctx context.Context, payload any) (retry bool, err error) {
	resp, err := n.client.POST("").
		Body().AsJSON(payload).
		Context().Set(ctx).
		Send()
	if err != nil {
		return true, errors.New("webhook delivery failed")
	}
	defer resp.Body().Close()
	if resp.Status().IsError() {
		return resp.Status().Is5xxServerError(),
			fmt.Errorf("webhook rejected: status %d", resp.Status().Code())
	}
	slog.Info("webhook delivered", "status", resp.Status().Code())
	return false, nil
}

// wait sleeps for d, returning early with the context error if ctx is done.
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
