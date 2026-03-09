package store

import (
	"context"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
)

func TestGetOpsRevision_InitialZero(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	rev, err := s.GetOpsRevision(ctx, RevTableAlerts)
	if err != nil {
		t.Fatalf("GetOpsRevision: %v", err)
	}
	if rev != 0 {
		t.Fatalf("initial rev = %d, want 0", rev)
	}
}

func TestGetOpsRevision_UnknownTable(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	rev, err := s.GetOpsRevision(ctx, "nonexistent_table")
	if err != nil {
		t.Fatalf("GetOpsRevision: %v", err)
	}
	if rev != 0 {
		t.Fatalf("rev for unknown table = %d, want 0", rev)
	}
}

func TestBumpOpsRevision_Increments(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		table   string
		wantRev int64
	}{
		{"first bump", RevTableAlerts, 1},
		{"second bump", RevTableAlerts, 2},
		{"third bump", RevTableAlerts, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev, err := s.BumpOpsRevision(ctx, tt.table)
			if err != nil {
				t.Fatalf("BumpOpsRevision: %v", err)
			}
			if rev != tt.wantRev {
				t.Fatalf("rev = %d, want %d", rev, tt.wantRev)
			}
		})
	}
}

func TestBumpOpsRevision_IndependentTables(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Bump alerts twice.
	if _, err := s.BumpOpsRevision(ctx, RevTableAlerts); err != nil {
		t.Fatalf("BumpOpsRevision(alerts): %v", err)
	}
	if _, err := s.BumpOpsRevision(ctx, RevTableAlerts); err != nil {
		t.Fatalf("BumpOpsRevision(alerts): %v", err)
	}

	// Bump activity once.
	actRev, err := s.BumpOpsRevision(ctx, RevTableActivity)
	if err != nil {
		t.Fatalf("BumpOpsRevision(activity): %v", err)
	}
	if actRev != 1 {
		t.Fatalf("activity rev = %d, want 1", actRev)
	}

	// Alerts should be at 2.
	alertRev, err := s.GetOpsRevision(ctx, RevTableAlerts)
	if err != nil {
		t.Fatalf("GetOpsRevision(alerts): %v", err)
	}
	if alertRev != 2 {
		t.Fatalf("alerts rev = %d, want 2", alertRev)
	}
}

func TestBumpOpsRevision_NewTable(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	rev, err := s.BumpOpsRevision(ctx, "custom_table")
	if err != nil {
		t.Fatalf("BumpOpsRevision(new): %v", err)
	}
	if rev != 1 {
		t.Fatalf("rev = %d, want 1", rev)
	}
}

func TestUpsertAlert_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	revBefore, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}

	if _, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "rev:test",
		Source:    "test",
		Title:     "Test Alert",
		Severity:  "warn",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}

	revAfter, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase: before=%d after=%d", revBefore, revAfter)
	}
}

func TestAckAlert_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "ack:rev",
		Source:    "test",
		Title:     "Ack Rev",
		Severity:  "error",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}

	revBefore, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}

	if _, err := s.AckAlert(ctx, alert.ID, base.Add(time.Minute)); err != nil {
		t.Fatalf("AckAlert: %v", err)
	}

	revAfter, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase: before=%d after=%d", revBefore, revAfter)
	}
}

func TestResolveAlert_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	if _, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "resolve:rev",
		Source:    "test",
		Title:     "Resolve Rev",
		Severity:  "error",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}

	revBefore, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}

	if _, err := s.ResolveAlert(ctx, "resolve:rev", base.Add(time.Minute)); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	revAfter, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase: before=%d after=%d", revBefore, revAfter)
	}
}

func TestDeleteAlert_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	alert, err := s.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "delete:rev",
		Source:    "test",
		Title:     "Delete Rev",
		Severity:  "error",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}
	if _, err := s.ResolveAlert(ctx, "delete:rev", base.Add(time.Minute)); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	revBefore, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}

	if err := s.DeleteAlert(ctx, alert.ID); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	revAfter, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase: before=%d after=%d", revBefore, revAfter)
	}
}

func TestInsertActivityEvent_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	revBefore, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}

	if _, err := s.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "test",
		EventType: "test.event",
		Severity:  "info",
		Resource:  "res",
		Message:   "test event",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("InsertActivityEvent: %v", err)
	}

	revAfter, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase: before=%d after=%d", revBefore, revAfter)
	}
}

func TestPruneOpsActivityRows_BumpsRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// Insert enough events to prune.
	for i := range 5 {
		if _, err := s.InsertActivityEvent(ctx, activity.EventWrite{
			Source:    "test",
			EventType: "test.event",
			Severity:  "info",
			Resource:  "res",
			Message:   "event",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("InsertActivityEvent(%d): %v", i, err)
		}
	}

	revBefore, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}

	n, err := s.PruneOpsActivityRows(ctx, 2)
	if err != nil {
		t.Fatalf("PruneOpsActivityRows: %v", err)
	}
	if n != 3 {
		t.Fatalf("pruned = %d, want 3", n)
	}

	revAfter, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}
	if revAfter <= revBefore {
		t.Fatalf("rev did not increase after prune: before=%d after=%d", revBefore, revAfter)
	}
}

func TestPruneOpsActivityRows_NoPrune_NoRevBump(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	revBefore, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}

	// Prune on empty table should not bump rev.
	n, err := s.PruneOpsActivityRows(ctx, 100)
	if err != nil {
		t.Fatalf("PruneOpsActivityRows: %v", err)
	}
	if n != 0 {
		t.Fatalf("pruned = %d, want 0", n)
	}

	revAfter, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision: %v", err)
	}
	if revAfter != revBefore {
		t.Fatalf("rev changed when nothing was pruned: before=%d after=%d", revBefore, revAfter)
	}
}

func TestAlertRevision_UnchangedSkipsDelta(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Read revision twice without mutation — should be identical.
	rev1, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision(1): %v", err)
	}
	rev2, err := s.GetOpsAlertRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsAlertRevision(2): %v", err)
	}
	if rev1 != rev2 {
		t.Fatalf("rev changed without mutation: %d != %d", rev1, rev2)
	}
}

func TestActivityRevision_UnchangedSkipsDelta(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	rev1, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision(1): %v", err)
	}
	rev2, err := s.GetOpsActivityRevision(ctx)
	if err != nil {
		t.Fatalf("GetOpsActivityRevision(2): %v", err)
	}
	if rev1 != rev2 {
		t.Fatalf("rev changed without mutation: %d != %d", rev1, rev2)
	}
}
