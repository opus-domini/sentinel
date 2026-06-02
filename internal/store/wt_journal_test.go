package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestWatchtowerJournalListFilteringLimitAndPrune(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	for _, row := range []WatchtowerJournalWrite{
		{GlobalRev: 1, EntityType: " session ", Session: " old ", WindowIdx: -1, ChangeKind: "created", ChangedAt: base},
		{GlobalRev: 2, EntityType: "pane", Session: "dev", WindowIdx: 0, PaneID: " %1 ", ChangeKind: "updated", ChangedAt: base.Add(time.Second)},
		{GlobalRev: 2, EntityType: "window", Session: "prod", WindowIdx: 1, ChangeKind: "updated", ChangedAt: base.Add(2 * time.Second)},
		{GlobalRev: 3, EntityType: "session", Session: "qa", WindowIdx: -1, ChangeKind: "deleted", ChangedAt: base.Add(3 * time.Second)},
	} {
		if _, err := s.InsertWatchtowerJournal(ctx, row); err != nil {
			t.Fatalf("InsertWatchtowerJournal(%d): %v", row.GlobalRev, err)
		}
	}

	rows, err := s.ListWatchtowerJournalSince(ctx, 1, 2)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince: %v", err)
	}
	gotRevs := []int64{rows[0].GlobalRev, rows[1].GlobalRev}
	if !reflect.DeepEqual(gotRevs, []int64{2, 2}) {
		t.Fatalf("revisions = %v, want [2 2]", gotRevs)
	}
	if rows[0].Session != "dev" || rows[0].PaneID != "%1" || !rows[0].ChangedAt.Equal(base.Add(time.Second)) {
		t.Fatalf("first limited row not trimmed/parsed as expected: %+v", rows[0])
	}

	affected, err := s.PruneWatchtowerJournalRows(ctx, 2)
	if err != nil {
		t.Fatalf("PruneWatchtowerJournalRows: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}
	remaining, err := s.ListWatchtowerJournalSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince remaining: %v", err)
	}
	gotSessions := []string{remaining[0].Session, remaining[1].Session}
	if !reflect.DeepEqual(gotSessions, []string{"prod", "qa"}) {
		t.Fatalf("remaining sessions = %v, want [prod qa]", gotSessions)
	}
}

func TestWatchtowerGlobalRevisionMissingValidAndInvalid(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	rev, err := s.WatchtowerGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("WatchtowerGlobalRevision missing: %v", err)
	}
	if rev != 0 {
		t.Fatalf("missing rev = %d, want 0", rev)
	}

	if err := s.SetWatchtowerRuntimeValue(ctx, "global_rev", " 123 "); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue valid: %v", err)
	}
	rev, err = s.WatchtowerGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("WatchtowerGlobalRevision valid: %v", err)
	}
	if rev != 123 {
		t.Fatalf("valid rev = %d, want 123", rev)
	}

	if err := s.SetWatchtowerRuntimeValue(ctx, "global_rev", "not-a-number"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue invalid: %v", err)
	}
	rev, err = s.WatchtowerGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("WatchtowerGlobalRevision invalid: %v", err)
	}
	if rev != 0 {
		t.Fatalf("invalid rev = %d, want 0", rev)
	}
}
