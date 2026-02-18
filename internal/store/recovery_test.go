package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

const testSessionDev = "dev"

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func validSnapshotWrite(session string) RecoverySnapshotWrite {
	return RecoverySnapshotWrite{
		SessionName:  session,
		BootID:       "boot-1",
		StateHash:    "hash-1",
		CapturedAt:   time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		ActiveWindow: 0,
		ActivePaneID: "%0",
		Windows:      2,
		Panes:        3,
		PayloadJSON:  `{"windows":[]}`,
	}
}

func mustUpsertSnapshot(t *testing.T, s *Store, ctx context.Context, snap RecoverySnapshotWrite) RecoverySnapshot {
	t.Helper()
	row, _, err := s.UpsertRecoverySnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("UpsertRecoverySnapshot(%s) error = %v", snap.SessionName, err)
	}
	return row
}

func mustCreateJob(t *testing.T, s *Store, ctx context.Context, job RecoveryJob) {
	t.Helper()
	if err := s.CreateRecoveryJob(ctx, job); err != nil {
		t.Fatalf("CreateRecoveryJob(%s) error = %v", job.ID, err)
	}
}

// ---------------------------------------------------------------------------
// UpsertRecoverySnapshot
// ---------------------------------------------------------------------------

func TestUpsertRecoverySnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("new snapshot returns isNew=true", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := validSnapshotWrite(testSessionDev)
		row, isNew, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("UpsertRecoverySnapshot error = %v", err)
		}
		if !isNew {
			t.Fatal("expected isNew=true for first snapshot")
		}
		if row.ID == 0 {
			t.Fatal("expected non-zero snapshot ID")
		}
		if row.SessionName != testSessionDev || row.StateHash != "hash-1" {
			t.Fatalf("unexpected snapshot: %+v", row)
		}
		if row.Windows != 2 || row.Panes != 3 {
			t.Fatalf("unexpected window/pane counts: %+v", row)
		}
	})

	t.Run("reuse path when hash unchanged", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := validSnapshotWrite(testSessionDev)
		first, isNew, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("first upsert error = %v", err)
		}
		if !isNew {
			t.Fatal("first should be new")
		}

		// Same hash → should reuse.
		snap.CapturedAt = snap.CapturedAt.Add(time.Minute)
		second, isNew, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("second upsert error = %v", err)
		}
		if isNew {
			t.Fatal("expected isNew=false for same hash")
		}
		if second.ID != first.ID {
			t.Fatalf("expected reused snapshot ID %d, got %d", first.ID, second.ID)
		}
	})

	t.Run("new snapshot when hash changes", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := validSnapshotWrite(testSessionDev)
		first, _, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("first upsert error = %v", err)
		}

		snap.StateHash = "hash-2"
		snap.CapturedAt = snap.CapturedAt.Add(time.Minute)
		second, isNew, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("second upsert error = %v", err)
		}
		if !isNew {
			t.Fatal("expected isNew=true for different hash")
		}
		if second.ID == first.ID {
			t.Fatal("expected different snapshot ID for different hash")
		}
	})

	t.Run("reuse with empty hash always creates new", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := validSnapshotWrite(testSessionDev)
		snap.StateHash = ""
		_, isNew1, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("first upsert error = %v", err)
		}
		if !isNew1 {
			t.Fatal("expected isNew=true for first")
		}

		// Empty hash → no dedup, always new.
		_, isNew2, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("second upsert error = %v", err)
		}
		if !isNew2 {
			t.Fatal("expected isNew=true for empty hash (no dedup)")
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		tests := []struct {
			name string
			snap RecoverySnapshotWrite
		}{
			{"empty session name", RecoverySnapshotWrite{SessionName: "", PayloadJSON: `{}`}},
			{"whitespace session name", RecoverySnapshotWrite{SessionName: "  ", PayloadJSON: `{}`}},
			{"empty payload", RecoverySnapshotWrite{SessionName: testSessionDev, PayloadJSON: ""}},
			{"whitespace payload", RecoverySnapshotWrite{SessionName: testSessionDev, PayloadJSON: "   "}},
			{"invalid JSON payload", RecoverySnapshotWrite{SessionName: testSessionDev, PayloadJSON: "not-json"}},
		}

		for _, tt := range tests {
			_, _, err := s.UpsertRecoverySnapshot(ctx, tt.snap)
			if err == nil {
				t.Errorf("%s: expected error, got nil", tt.name)
			}
		}
	})

	t.Run("zero capturedAt defaults to now", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := validSnapshotWrite(testSessionDev)
		snap.CapturedAt = time.Time{}
		row, _, err := s.UpsertRecoverySnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("UpsertRecoverySnapshot error = %v", err)
		}
		if row.CapturedAt.IsZero() {
			t.Fatal("expected non-zero CapturedAt when input is zero")
		}
	})
}

// ---------------------------------------------------------------------------
// GetRecoverySnapshot
// ---------------------------------------------------------------------------

func TestGetRecoverySnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("existing snapshot", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		created := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))

		got, err := s.GetRecoverySnapshot(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetRecoverySnapshot error = %v", err)
		}
		if got.ID != created.ID || got.SessionName != testSessionDev {
			t.Fatalf("unexpected snapshot: %+v", got)
		}
		if got.PayloadJSON != `{"windows":[]}` {
			t.Fatalf("payload = %q, want %q", got.PayloadJSON, `{"windows":[]}`)
		}
	})

	t.Run("not found", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		_, err := s.GetRecoverySnapshot(ctx, 999)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// ListRecoverySnapshots
// ---------------------------------------------------------------------------

func TestListRecoverySnapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns snapshots ordered by captured_at desc", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		base := validSnapshotWrite(testSessionDev)
		// Insert 3 snapshots with different hashes so no dedup.
		for i := 0; i < 3; i++ {
			snap := base
			snap.StateHash = ""
			snap.CapturedAt = base.CapturedAt.Add(time.Duration(i) * time.Hour)
			mustUpsertSnapshot(t, s, ctx, snap)
		}

		list, err := s.ListRecoverySnapshots(ctx, testSessionDev, 10)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("got %d snapshots, want 3", len(list))
		}
		// First should be the most recent.
		if !list[0].CapturedAt.After(list[1].CapturedAt) {
			t.Fatalf("snapshots not ordered by captured_at DESC")
		}
	})

	t.Run("limit works", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		base := validSnapshotWrite(testSessionDev)
		for i := 0; i < 5; i++ {
			snap := base
			snap.StateHash = ""
			snap.CapturedAt = base.CapturedAt.Add(time.Duration(i) * time.Hour)
			mustUpsertSnapshot(t, s, ctx, snap)
		}

		list, err := s.ListRecoverySnapshots(ctx, testSessionDev, 2)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("got %d snapshots, want 2", len(list))
		}
	})

	t.Run("default limit when zero", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		// Just verify it doesn't error.
		list, err := s.ListRecoverySnapshots(ctx, testSessionDev, 0)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %d snapshots, want 0 for empty session", len(list))
		}
	})

	t.Run("empty for unknown session", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		list, err := s.ListRecoverySnapshots(ctx, "nonexistent", 10)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %d snapshots, want 0", len(list))
		}
	})
}

// ---------------------------------------------------------------------------
// ListRecoverySessions / GetRecoverySession
// ---------------------------------------------------------------------------

func TestListRecoverySessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("lists all sessions", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("alpha"))
		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("beta"))

		list, err := s.ListRecoverySessions(ctx, nil)
		if err != nil {
			t.Fatalf("ListRecoverySessions error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("got %d sessions, want 2", len(list))
		}
	})

	t.Run("filter by state", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("alive"))
		snap := validSnapshotWrite("dead")
		mustUpsertSnapshot(t, s, ctx, snap)
		if err := s.MarkRecoverySessionsKilled(ctx, []string{"dead"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		running, err := s.ListRecoverySessions(ctx, []RecoverySessionState{RecoveryStateRunning})
		if err != nil {
			t.Fatalf("ListRecoverySessions(running) error = %v", err)
		}
		if len(running) != 1 || running[0].Name != "alive" {
			t.Fatalf("expected only 'alive', got %+v", running)
		}

		killed, err := s.ListRecoverySessions(ctx, []RecoverySessionState{RecoveryStateKilled})
		if err != nil {
			t.Fatalf("ListRecoverySessions(killed) error = %v", err)
		}
		if len(killed) != 1 || killed[0].Name != "dead" {
			t.Fatalf("expected only 'dead', got %+v", killed)
		}
	})

	t.Run("filter by multiple states", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("running-1"))
		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("killed-1"))
		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("archived-1"))

		if err := s.MarkRecoverySessionsKilled(ctx, []string{"killed-1"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}
		if err := s.MarkRecoverySessionArchived(ctx, "archived-1", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionArchived error = %v", err)
		}

		results, err := s.ListRecoverySessions(ctx, []RecoverySessionState{RecoveryStateKilled, RecoveryStateArchived})
		if err != nil {
			t.Fatalf("ListRecoverySessions error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("got %d sessions, want 2", len(results))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		list, err := s.ListRecoverySessions(ctx, nil)
		if err != nil {
			t.Fatalf("ListRecoverySessions error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %d sessions, want 0", len(list))
		}
	})
}

func TestGetRecoverySession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("existing session", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))

		session, err := s.GetRecoverySession(ctx, testSessionDev)
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.Name != testSessionDev || session.State != RecoveryStateRunning {
			t.Fatalf("unexpected session: %+v", session)
		}
		if session.SnapshotHash != "hash-1" || session.Windows != 2 || session.Panes != 3 {
			t.Fatalf("unexpected session data: %+v", session)
		}
		if session.SnapshotAt.IsZero() || session.LastSeenAt.IsZero() {
			t.Fatalf("expected non-zero timestamps: %+v", session)
		}
	})

	t.Run("not found", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		_, err := s.GetRecoverySession(ctx, "nonexistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// MarkRecoverySessionsKilled
// ---------------------------------------------------------------------------

func TestMarkRecoverySessionsKilled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("marks running sessions as killed", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("s1"))
		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("s2"))

		killedAt := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
		if err := s.MarkRecoverySessionsKilled(ctx, []string{"s1", "s2"}, "boot-2", killedAt); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		for _, name := range []string{"s1", "s2"} {
			session, err := s.GetRecoverySession(ctx, name)
			if err != nil {
				t.Fatalf("GetRecoverySession(%s) error = %v", name, err)
			}
			if session.State != RecoveryStateKilled {
				t.Fatalf("%s state = %s, want killed", name, session.State)
			}
			if session.KilledAt == nil {
				t.Fatalf("%s KilledAt is nil", name)
			}
			if session.LastBootID != "boot-2" {
				t.Fatalf("%s LastBootID = %q, want boot-2", name, session.LastBootID)
			}
		}
	})

	t.Run("empty names is noop", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.MarkRecoverySessionsKilled(ctx, []string{}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}
	})

	t.Run("does not mark archived sessions", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("arch"))
		if err := s.MarkRecoverySessionArchived(ctx, "arch", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionArchived error = %v", err)
		}

		if err := s.MarkRecoverySessionsKilled(ctx, []string{"arch"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "arch")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateArchived {
			t.Fatalf("archived session state = %s, want archived", session.State)
		}
	})

	t.Run("skips blank names", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("valid"))
		if err := s.MarkRecoverySessionsKilled(ctx, []string{"", "  ", "valid"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "valid")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateKilled {
			t.Fatalf("state = %s, want killed", session.State)
		}
	})
}

// ---------------------------------------------------------------------------
// RenameRecoverySession
// ---------------------------------------------------------------------------

func TestRenameRecoverySession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("renames session and snapshots", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("old"))

		if err := s.RenameRecoverySession(ctx, "old", "new"); err != nil {
			t.Fatalf("RenameRecoverySession error = %v", err)
		}

		// Old name should not exist.
		_, err := s.GetRecoverySession(ctx, "old")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for old name, got %v", err)
		}

		// New name should exist.
		session, err := s.GetRecoverySession(ctx, "new")
		if err != nil {
			t.Fatalf("GetRecoverySession(new) error = %v", err)
		}
		if session.Name != "new" {
			t.Fatalf("session name = %q, want new", session.Name)
		}

		// Snapshots should also be renamed.
		snaps, err := s.ListRecoverySnapshots(ctx, "new", 10)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots(new) error = %v", err)
		}
		if len(snaps) != 1 {
			t.Fatalf("got %d snapshots for new, want 1", len(snaps))
		}

		oldSnaps, err := s.ListRecoverySnapshots(ctx, "old", 10)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots(old) error = %v", err)
		}
		if len(oldSnaps) != 0 {
			t.Fatalf("got %d snapshots for old, want 0", len(oldSnaps))
		}
	})

	t.Run("also renames jobs", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("src"))
		mustCreateJob(t, s, ctx, RecoveryJob{
			ID:             "job-1",
			SessionName:    "src",
			SnapshotID:     snap.ID,
			Mode:           "full",
			ConflictPolicy: "rename",
			Status:         RecoveryJobQueued,
			CreatedAt:      time.Now(),
		})

		if err := s.RenameRecoverySession(ctx, "src", "dst"); err != nil {
			t.Fatalf("RenameRecoverySession error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.SessionName != "dst" {
			t.Fatalf("job session = %q, want dst", job.SessionName)
		}
	})

	t.Run("noop cases", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		// Empty old name.
		if err := s.RenameRecoverySession(ctx, "", "new"); err != nil {
			t.Fatalf("empty old error = %v", err)
		}
		// Empty new name.
		if err := s.RenameRecoverySession(ctx, "old", ""); err != nil {
			t.Fatalf("empty new error = %v", err)
		}
		// Same name.
		if err := s.RenameRecoverySession(ctx, "same", "same"); err != nil {
			t.Fatalf("same name error = %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// State transitions: Archive / Restoring / Restored / RestoreFailed
// ---------------------------------------------------------------------------

func TestRecoverySessionStateTransitions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("mark archived", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("sess"))

		archivedAt := time.Date(2025, 6, 3, 0, 0, 0, 0, time.UTC)
		if err := s.MarkRecoverySessionArchived(ctx, "sess", archivedAt); err != nil {
			t.Fatalf("MarkRecoverySessionArchived error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "sess")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateArchived {
			t.Fatalf("state = %s, want archived", session.State)
		}
		if session.ArchivedAt == nil {
			t.Fatal("ArchivedAt is nil")
		}
	})

	t.Run("mark restoring", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("sess"))
		if err := s.MarkRecoverySessionsKilled(ctx, []string{"sess"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		if err := s.MarkRecoverySessionRestoring(ctx, "sess"); err != nil {
			t.Fatalf("MarkRecoverySessionRestoring error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "sess")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateRestoring {
			t.Fatalf("state = %s, want restoring", session.State)
		}
		if session.RestoreError != "" {
			t.Fatalf("restore error should be cleared, got %q", session.RestoreError)
		}
	})

	t.Run("mark restored", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("sess"))
		if err := s.MarkRecoverySessionsKilled(ctx, []string{"sess"}, "boot-2", time.Now()); err != nil {
			t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
		}

		restoredAt := time.Date(2025, 6, 4, 0, 0, 0, 0, time.UTC)
		if err := s.MarkRecoverySessionRestored(ctx, "sess", restoredAt); err != nil {
			t.Fatalf("MarkRecoverySessionRestored error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "sess")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateRestored {
			t.Fatalf("state = %s, want restored", session.State)
		}
		if session.RestoredAt == nil {
			t.Fatal("RestoredAt is nil")
		}
		// killed_at should be cleared.
		if session.KilledAt != nil {
			t.Fatal("KilledAt should be cleared after restore")
		}
	})

	t.Run("mark restore failed", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("sess"))
		if err := s.MarkRecoverySessionRestoring(ctx, "sess"); err != nil {
			t.Fatalf("MarkRecoverySessionRestoring error = %v", err)
		}

		if err := s.MarkRecoverySessionRestoreFailed(ctx, "sess", "tmux server died"); err != nil {
			t.Fatalf("MarkRecoverySessionRestoreFailed error = %v", err)
		}

		session, err := s.GetRecoverySession(ctx, "sess")
		if err != nil {
			t.Fatalf("GetRecoverySession error = %v", err)
		}
		if session.State != RecoveryStateKilled {
			t.Fatalf("state = %s, want killed (after failed restore)", session.State)
		}
		if session.RestoreError != "tmux server died" {
			t.Fatalf("restore error = %q, want %q", session.RestoreError, "tmux server died")
		}
	})
}

// ---------------------------------------------------------------------------
// TrimRecoverySnapshots
// ---------------------------------------------------------------------------

func TestTrimRecoverySnapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("trims excess snapshots per session", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		base := validSnapshotWrite(testSessionDev)
		for i := 0; i < 5; i++ {
			snap := base
			snap.StateHash = "" // no dedup
			snap.CapturedAt = base.CapturedAt.Add(time.Duration(i) * time.Hour)
			mustUpsertSnapshot(t, s, ctx, snap)
		}

		if err := s.TrimRecoverySnapshots(ctx, 2); err != nil {
			t.Fatalf("TrimRecoverySnapshots error = %v", err)
		}

		remaining, err := s.ListRecoverySnapshots(ctx, testSessionDev, 100)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(remaining) != 2 {
			t.Fatalf("got %d snapshots, want 2", len(remaining))
		}
	})

	t.Run("noop when maxPerSession is zero", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.TrimRecoverySnapshots(ctx, 0); err != nil {
			t.Fatalf("TrimRecoverySnapshots(0) error = %v", err)
		}
	})

	t.Run("noop when fewer than max", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))

		if err := s.TrimRecoverySnapshots(ctx, 10); err != nil {
			t.Fatalf("TrimRecoverySnapshots error = %v", err)
		}

		remaining, err := s.ListRecoverySnapshots(ctx, testSessionDev, 100)
		if err != nil {
			t.Fatalf("ListRecoverySnapshots error = %v", err)
		}
		if len(remaining) != 1 {
			t.Fatalf("got %d snapshots, want 1", len(remaining))
		}
	})

	t.Run("trims per session independently", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		for _, session := range []string{"a", "b"} {
			base := validSnapshotWrite(session)
			for i := 0; i < 4; i++ {
				snap := base
				snap.StateHash = ""
				snap.CapturedAt = base.CapturedAt.Add(time.Duration(i) * time.Hour)
				mustUpsertSnapshot(t, s, ctx, snap)
			}
		}

		if err := s.TrimRecoverySnapshots(ctx, 1); err != nil {
			t.Fatalf("TrimRecoverySnapshots error = %v", err)
		}

		for _, session := range []string{"a", "b"} {
			remaining, err := s.ListRecoverySnapshots(ctx, session, 100)
			if err != nil {
				t.Fatalf("ListRecoverySnapshots(%s) error = %v", session, err)
			}
			if len(remaining) != 1 {
				t.Fatalf("session %s: got %d snapshots, want 1", session, len(remaining))
			}
		}
	})
}

// ---------------------------------------------------------------------------
// SetRuntimeValue / GetRuntimeValue
// ---------------------------------------------------------------------------

func TestRuntimeValues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("set and get", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.SetRuntimeValue(ctx, "boot_id", "abc-123"); err != nil {
			t.Fatalf("SetRuntimeValue error = %v", err)
		}

		got, err := s.GetRuntimeValue(ctx, "boot_id")
		if err != nil {
			t.Fatalf("GetRuntimeValue error = %v", err)
		}
		if got != "abc-123" {
			t.Fatalf("value = %q, want abc-123", got)
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.SetRuntimeValue(ctx, "key", "v1"); err != nil {
			t.Fatalf("SetRuntimeValue error = %v", err)
		}
		if err := s.SetRuntimeValue(ctx, "key", "v2"); err != nil {
			t.Fatalf("SetRuntimeValue (update) error = %v", err)
		}

		got, err := s.GetRuntimeValue(ctx, "key")
		if err != nil {
			t.Fatalf("GetRuntimeValue error = %v", err)
		}
		if got != "v2" {
			t.Fatalf("value = %q, want v2", got)
		}
	})

	t.Run("missing key returns empty string", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		got, err := s.GetRuntimeValue(ctx, "missing")
		if err != nil {
			t.Fatalf("GetRuntimeValue error = %v", err)
		}
		if got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Recovery Jobs: Create, Get, List, state transitions
// ---------------------------------------------------------------------------

func TestCreateAndGetRecoveryJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("create and retrieve", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		createdAt := time.Date(2025, 6, 5, 12, 0, 0, 0, time.UTC)

		mustCreateJob(t, s, ctx, RecoveryJob{
			ID:             "job-1",
			SessionName:    testSessionDev,
			TargetSession:  "dev-restored",
			SnapshotID:     snap.ID,
			Mode:           "full",
			ConflictPolicy: "rename",
			Status:         RecoveryJobQueued,
			TotalSteps:     5,
			CreatedAt:      createdAt,
		})

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.ID != "job-1" || job.SessionName != testSessionDev {
			t.Fatalf("unexpected job identity: %+v", job)
		}
		if job.Status != RecoveryJobQueued {
			t.Fatalf("status = %s, want queued", job.Status)
		}
		if job.TotalSteps != 5 || job.CompletedSteps != 0 {
			t.Fatalf("unexpected step counts: %+v", job)
		}
		if job.TargetSession != "dev-restored" {
			t.Fatalf("target = %q, want dev-restored", job.TargetSession)
		}
		if job.StartedAt != nil || job.FinishedAt != nil {
			t.Fatalf("expected nil timestamps for queued job: %+v", job)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		_, err := s.GetRecoveryJob(ctx, "nonexistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("create with zero createdAt defaults to now", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		mustCreateJob(t, s, ctx, RecoveryJob{
			ID:             "job-z",
			SessionName:    testSessionDev,
			SnapshotID:     snap.ID,
			Mode:           "full",
			ConflictPolicy: "rename",
			Status:         RecoveryJobQueued,
		})

		job, err := s.GetRecoveryJob(ctx, "job-z")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.CreatedAt.IsZero() {
			t.Fatal("expected non-zero CreatedAt")
		}
	})
}

func TestRecoveryJobStateTransitions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Helper to create a queued job in a fresh store.
	setupJob := func(t *testing.T) (*Store, RecoveryJob) {
		t.Helper()
		s := newTestStore(t)
		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		job := RecoveryJob{
			ID:             "job-1",
			SessionName:    testSessionDev,
			SnapshotID:     snap.ID,
			Mode:           "full",
			ConflictPolicy: "rename",
			Status:         RecoveryJobQueued,
			TotalSteps:     5,
			CreatedAt:      time.Now().UTC(),
		}
		mustCreateJob(t, s, ctx, job)
		return s, job
	}

	t.Run("set running", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		startedAt := time.Date(2025, 6, 5, 12, 1, 0, 0, time.UTC)
		if err := s.SetRecoveryJobRunning(ctx, "job-1", startedAt); err != nil {
			t.Fatalf("SetRecoveryJobRunning error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.Status != RecoveryJobRunning {
			t.Fatalf("status = %s, want running", job.Status)
		}
		if job.StartedAt == nil {
			t.Fatal("StartedAt is nil")
		}
		if job.Error != "" || job.CurrentStep != "" {
			t.Fatalf("running job should have cleared error/step: %+v", job)
		}
	})

	t.Run("update target", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		if err := s.UpdateRecoveryJobTarget(ctx, "job-1", "  new-target  "); err != nil {
			t.Fatalf("UpdateRecoveryJobTarget error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.TargetSession != "new-target" {
			t.Fatalf("target = %q, want new-target", job.TargetSession)
		}
	})

	t.Run("update progress", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		if err := s.UpdateRecoveryJobProgress(ctx, "job-1", 3, 5, "creating windows"); err != nil {
			t.Fatalf("UpdateRecoveryJobProgress error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.CompletedSteps != 3 || job.TotalSteps != 5 {
			t.Fatalf("steps = %d/%d, want 3/5", job.CompletedSteps, job.TotalSteps)
		}
		if job.CurrentStep != "creating windows" {
			t.Fatalf("current step = %q, want %q", job.CurrentStep, "creating windows")
		}
	})

	t.Run("finish succeeded", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		finishedAt := time.Date(2025, 6, 5, 12, 5, 0, 0, time.UTC)
		if err := s.FinishRecoveryJob(ctx, "job-1", RecoveryJobSucceeded, "", finishedAt); err != nil {
			t.Fatalf("FinishRecoveryJob error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.Status != RecoveryJobSucceeded {
			t.Fatalf("status = %s, want succeeded", job.Status)
		}
		if job.FinishedAt == nil {
			t.Fatal("FinishedAt is nil")
		}
		if job.Error != "" {
			t.Fatalf("error = %q, want empty", job.Error)
		}
		if job.CurrentStep != "" {
			t.Fatalf("current step should be cleared, got %q", job.CurrentStep)
		}
	})

	t.Run("finish failed", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		finishedAt := time.Date(2025, 6, 5, 12, 5, 0, 0, time.UTC)
		if err := s.FinishRecoveryJob(ctx, "job-1", RecoveryJobFailed, "tmux error", finishedAt); err != nil {
			t.Fatalf("FinishRecoveryJob error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.Status != RecoveryJobFailed {
			t.Fatalf("status = %s, want failed", job.Status)
		}
		if job.Error != "tmux error" {
			t.Fatalf("error = %q, want %q", job.Error, "tmux error")
		}
	})

	t.Run("finish partial", func(t *testing.T) {
		s, _ := setupJob(t)
		defer func() { _ = s.Close() }()

		finishedAt := time.Date(2025, 6, 5, 12, 5, 0, 0, time.UTC)
		if err := s.FinishRecoveryJob(ctx, "job-1", RecoveryJobPartial, "2 of 5 failed", finishedAt); err != nil {
			t.Fatalf("FinishRecoveryJob error = %v", err)
		}

		job, err := s.GetRecoveryJob(ctx, "job-1")
		if err != nil {
			t.Fatalf("GetRecoveryJob error = %v", err)
		}
		if job.Status != RecoveryJobPartial {
			t.Fatalf("status = %s, want partial", job.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// ListRecoveryJobs
// ---------------------------------------------------------------------------

func TestListRecoveryJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("list all with default limit", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		for i := 0; i < 3; i++ {
			mustCreateJob(t, s, ctx, RecoveryJob{
				ID:             "job-" + string(rune('a'+i)),
				SessionName:    testSessionDev,
				SnapshotID:     snap.ID,
				Mode:           "full",
				ConflictPolicy: "rename",
				Status:         RecoveryJobQueued,
				CreatedAt:      time.Date(2025, 6, 5, 12, i, 0, 0, time.UTC),
			})
		}

		list, err := s.ListRecoveryJobs(ctx, nil, 0)
		if err != nil {
			t.Fatalf("ListRecoveryJobs error = %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("got %d jobs, want 3", len(list))
		}
		// Should be ordered by created_at DESC.
		if list[0].CreatedAt.Before(list[1].CreatedAt) {
			t.Fatalf("jobs not ordered by created_at DESC")
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		mustCreateJob(t, s, ctx, RecoveryJob{
			ID: "j1", SessionName: testSessionDev, SnapshotID: snap.ID,
			Mode: "full", ConflictPolicy: "rename", Status: RecoveryJobQueued,
			CreatedAt: time.Now(),
		})
		mustCreateJob(t, s, ctx, RecoveryJob{
			ID: "j2", SessionName: testSessionDev, SnapshotID: snap.ID,
			Mode: "full", ConflictPolicy: "rename", Status: RecoveryJobSucceeded,
			CreatedAt: time.Now(),
		})

		queued, err := s.ListRecoveryJobs(ctx, []RecoveryJobStatus{RecoveryJobQueued}, 10)
		if err != nil {
			t.Fatalf("ListRecoveryJobs error = %v", err)
		}
		if len(queued) != 1 || queued[0].ID != "j1" {
			t.Fatalf("expected only j1, got %+v", queued)
		}
	})

	t.Run("limit works", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		snap := mustUpsertSnapshot(t, s, ctx, validSnapshotWrite(testSessionDev))
		for i := 0; i < 5; i++ {
			mustCreateJob(t, s, ctx, RecoveryJob{
				ID: "j" + string(rune('0'+i)), SessionName: testSessionDev, SnapshotID: snap.ID,
				Mode: "full", ConflictPolicy: "rename", Status: RecoveryJobQueued,
				CreatedAt: time.Date(2025, 6, 5, 12, i, 0, 0, time.UTC),
			})
		}

		list, err := s.ListRecoveryJobs(ctx, nil, 2)
		if err != nil {
			t.Fatalf("ListRecoveryJobs error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("got %d jobs, want 2", len(list))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		list, err := s.ListRecoveryJobs(ctx, nil, 10)
		if err != nil {
			t.Fatalf("ListRecoveryJobs error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %d jobs, want 0", len(list))
		}
	})
}

// ---------------------------------------------------------------------------
// parseStoreTimePtr (indirectly tested via session timestamps)
// ---------------------------------------------------------------------------

func TestParseStoreTimePtr(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns nil", func(t *testing.T) {
		if got := parseStoreTimePtr(""); got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})

	t.Run("whitespace returns nil", func(t *testing.T) {
		if got := parseStoreTimePtr("  "); got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})

	t.Run("RFC3339 string", func(t *testing.T) {
		ts := parseStoreTimePtr("2025-06-01T12:00:00Z")
		if ts == nil {
			t.Fatal("expected non-nil")
			return
		}
		if ts.Year() != 2025 || ts.Month() != 6 || ts.Day() != 1 {
			t.Fatalf("unexpected time: %v", *ts)
		}
	})

	t.Run("datetime format string", func(t *testing.T) {
		ts := parseStoreTimePtr("2025-06-01 12:00:00")
		if ts == nil {
			t.Fatal("expected non-nil")
			return
		}
		if ts.Year() != 2025 || ts.Month() != 6 {
			t.Fatalf("unexpected time: %v", *ts)
		}
	})

	t.Run("unparseable returns nil", func(t *testing.T) {
		if got := parseStoreTimePtr("not-a-time"); got != nil {
			t.Fatalf("expected nil for unparseable, got %v", *got)
		}
	})
}

// ---------------------------------------------------------------------------
// Recovery session ordering in list
// ---------------------------------------------------------------------------

func TestListRecoverySessionsOrdering(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	// Create sessions in different states.
	mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("running-sess"))
	mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("killed-sess"))
	mustUpsertSnapshot(t, s, ctx, validSnapshotWrite("restored-sess"))

	if err := s.MarkRecoverySessionsKilled(ctx, []string{"killed-sess"}, "boot-2", time.Now()); err != nil {
		t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
	}
	if err := s.MarkRecoverySessionRestored(ctx, "restored-sess", time.Now()); err != nil {
		t.Fatalf("MarkRecoverySessionRestored error = %v", err)
	}

	list, err := s.ListRecoverySessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListRecoverySessions error = %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d sessions, want 3", len(list))
	}
	// Killed should come first per the ORDER BY CASE clause.
	if list[0].State != RecoveryStateKilled {
		t.Fatalf("first session state = %s, want killed", list[0].State)
	}
}

// ---------------------------------------------------------------------------
// Recovery session gets refreshed to running on re-upsert
// ---------------------------------------------------------------------------

func TestUpsertRecoverySnapshotResetsKilledSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	snap := validSnapshotWrite(testSessionDev)
	mustUpsertSnapshot(t, s, ctx, snap)

	if err := s.MarkRecoverySessionsKilled(ctx, []string{testSessionDev}, "boot-2", time.Now()); err != nil {
		t.Fatalf("MarkRecoverySessionsKilled error = %v", err)
	}

	session, err := s.GetRecoverySession(ctx, testSessionDev)
	if err != nil {
		t.Fatalf("GetRecoverySession error = %v", err)
	}
	if session.State != RecoveryStateKilled {
		t.Fatalf("state = %s, want killed", session.State)
	}

	// New snapshot with different hash → should reset to running.
	snap.StateHash = "hash-new"
	snap.CapturedAt = snap.CapturedAt.Add(time.Hour)
	mustUpsertSnapshot(t, s, ctx, snap)

	session, err = s.GetRecoverySession(ctx, testSessionDev)
	if err != nil {
		t.Fatalf("GetRecoverySession error = %v", err)
	}
	if session.State != RecoveryStateRunning {
		t.Fatalf("state = %s, want running (should reset after new snapshot)", session.State)
	}
	if session.KilledAt != nil {
		t.Fatal("KilledAt should be cleared after new snapshot")
	}
}
