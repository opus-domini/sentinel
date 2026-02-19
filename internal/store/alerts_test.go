package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
)

func TestOpsAlertUpsertAndAck(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	first, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertAlert(first): %v", err)
	}

	second, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed again",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertAlert(second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("alert id changed on dedupe: first=%d second=%d", first.ID, second.ID)
	}
	if second.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", second.Occurrences)
	}

	alertsList, err := s.ListAlerts(ctx, 10, alerts.StatusOpen)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alertsList) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alertsList))
	}

	acked, err := s.AckAlert(ctx, first.ID, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("AckAlert: %v", err)
	}
	if acked.Status != alerts.StatusAcked {
		t.Fatalf("status = %q, want %q", acked.Status, alerts.StatusAcked)
	}
	if acked.AckedAt == "" {
		t.Fatalf("ackedAt should not be empty")
	}

	_, err = s.AckAlert(ctx, 99999, base)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ack missing alert error = %v, want sql.ErrNoRows", err)
	}
}

func TestResolveOpsAlert(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed an open alert.
	alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "resolve:test",
		Source:    "test",
		Resource:  "svc",
		Title:     "Test Alert",
		Message:   "something went wrong",
		Severity:  "error",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}

	t.Run("resolve open alert", func(t *testing.T) {
		resolved, err := s.ResolveAlert(ctx, "resolve:test", base.Add(time.Minute))
		if err != nil {
			t.Fatalf("ResolveAlert: %v", err)
		}
		if resolved.Status != alerts.StatusResolved {
			t.Fatalf("status = %q, want %q", resolved.Status, alerts.StatusResolved)
		}
		if resolved.ResolvedAt == "" {
			t.Fatalf("resolvedAt should not be empty")
		}
		if resolved.ID != alert.ID {
			t.Fatalf("id = %d, want %d", resolved.ID, alert.ID)
		}
	})

	t.Run("resolve already resolved returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveAlert(ctx, "resolve:test", base.Add(2*time.Minute))
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("resolve empty dedupe key returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveAlert(ctx, "", base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("resolve nonexistent returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveAlert(ctx, "no:such:key", base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestUpsertOpsAlertReopen(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Create and resolve an alert.
	if _, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "reopen:test",
		Source:    "test",
		Title:     "Reopen Alert",
		Severity:  "warn",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("UpsertAlert(create): %v", err)
	}
	if _, err := s.ResolveAlert(ctx, "reopen:test", base.Add(time.Minute)); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	// Upsert same dedupe key → should reopen (status back to open).
	reopened, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "reopen:test",
		Source:    "test",
		Title:     "Reopen Alert",
		Severity:  "warn",
		CreatedAt: base.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertAlert(reopen): %v", err)
	}
	if reopened.Status != alerts.StatusOpen {
		t.Fatalf("status = %q, want %q (should reopen)", reopened.Status, alerts.StatusOpen)
	}
	if reopened.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", reopened.Occurrences)
	}
}

func TestUpsertOpsAlertValidation(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("empty dedupe key errors", func(t *testing.T) {
		_, err := s.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: "",
			Source:    "test",
			CreatedAt: base,
		})
		if err == nil {
			t.Fatalf("expected error for empty dedupe key")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: "defaults:test",
			CreatedAt: base,
		})
		if err != nil {
			t.Fatalf("UpsertAlert: %v", err)
		}
		// Source defaults to "ops".
		if alert.Source != "ops" {
			t.Fatalf("source = %q, want ops", alert.Source)
		}
		// Title defaults to dedupe key.
		if alert.Title != "defaults:test" {
			t.Fatalf("title = %q, want defaults:test", alert.Title)
		}
		// Message defaults to title.
		if alert.Message != "defaults:test" {
			t.Fatalf("message = %q, want defaults:test", alert.Message)
		}
		// Severity defaults to info.
		if alert.Severity != "info" {
			t.Fatalf("severity = %q, want info", alert.Severity)
		}
	})
}

func TestAckOpsAlertEdgeCases(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("ack negative ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.AckAlert(ctx, -1, base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("ack zero ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.AckAlert(ctx, 0, base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("cannot ack resolved alert", func(t *testing.T) {
		alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: "ack:resolved",
			Source:    "test",
			Title:     "Resolved Alert",
			Severity:  "error",
			CreatedAt: base,
		})
		if err != nil {
			t.Fatalf("UpsertAlert: %v", err)
		}
		if _, err := s.ResolveAlert(ctx, "ack:resolved", base.Add(time.Minute)); err != nil {
			t.Fatalf("ResolveAlert: %v", err)
		}

		_, err = s.AckAlert(ctx, alert.ID, base.Add(2*time.Minute))
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows (cannot ack resolved)", err)
		}
	})
}

func TestListOpsAlertsFilters(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("invalid status returns error", func(t *testing.T) {
		_, err := s.ListAlerts(ctx, 10, "bogus")
		if err == nil {
			t.Fatalf("expected error for invalid status")
		}
		if !errors.Is(err, alerts.ErrInvalidFilter) {
			t.Fatalf("error = %v, want alerts.ErrInvalidFilter", err)
		}
	})

	t.Run("empty status returns all", func(t *testing.T) {
		if _, err := s.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: "list:a",
			Source:    "test",
			Severity:  "info",
			CreatedAt: base,
		}); err != nil {
			t.Fatalf("UpsertAlert: %v", err)
		}

		alertsList, err := s.ListAlerts(ctx, 10, "")
		if err != nil {
			t.Fatalf("ListAlerts: %v", err)
		}
		if len(alertsList) < 1 {
			t.Fatalf("expected at least 1 alert")
		}
	})
}

func TestResolveOpsAlertAcked(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Create and ack an alert.
	alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "resolve:acked",
		Source:    "test",
		Title:     "Acked Alert",
		Severity:  "warn",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}
	if _, err := s.AckAlert(ctx, alert.ID, base.Add(time.Minute)); err != nil {
		t.Fatalf("AckAlert: %v", err)
	}

	// Resolving an acked alert should succeed.
	resolved, err := s.ResolveAlert(ctx, "resolve:acked", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ResolveAlert(acked): %v", err)
	}
	if resolved.Status != alerts.StatusResolved {
		t.Fatalf("status = %q, want %q", resolved.Status, alerts.StatusResolved)
	}
}
