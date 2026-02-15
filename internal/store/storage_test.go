package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStorageStatsAndFlush(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	if _, err := s.InsertWatchtowerTimelineEvent(ctx, WatchtowerTimelineEventWrite{
		Session:   "dev",
		WindowIdx: 0,
		PaneID:    "%1",
		EventType: "output.marker",
		Severity:  "warn",
		Summary:   "warning marker",
		Details:   "deprecated warning",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("InsertWatchtowerTimelineEvent: %v", err)
	}

	if _, err := s.InsertWatchtowerJournal(ctx, WatchtowerJournalWrite{
		GlobalRev:  1,
		EntityType: "pane",
		Session:    "dev",
		WindowIdx:  0,
		PaneID:     "%1",
		ChangeKind: "updated",
		ChangedAt:  base,
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal: %v", err)
	}

	if _, err := s.InsertGuardrailAudit(ctx, GuardrailAuditWrite{
		RuleID:      "rule.test",
		Decision:    "warn",
		Action:      "session.kill",
		Command:     "tmux kill-session -t dev",
		SessionName: "dev",
		WindowIndex: 0,
		PaneID:      "%1",
		Reason:      "test",
		MetadataRaw: `{"source":"test"}`,
		CreatedAt:   base,
	}); err != nil {
		t.Fatalf("InsertGuardrailAudit: %v", err)
	}

	snapshot, _, err := s.UpsertRecoverySnapshot(ctx, RecoverySnapshotWrite{
		SessionName:  "dev",
		BootID:       "boot-1",
		StateHash:    "hash-1",
		CapturedAt:   base,
		ActiveWindow: 0,
		ActivePaneID: "%1",
		Windows:      1,
		Panes:        1,
		PayloadJSON:  `{"windows":[],"panes":[]}`,
	})
	if err != nil {
		t.Fatalf("UpsertRecoverySnapshot: %v", err)
	}
	if err := s.CreateRecoveryJob(ctx, RecoveryJob{
		ID:             "job-1",
		SessionName:    "dev",
		TargetSession:  "dev-restored",
		SnapshotID:     snapshot.ID,
		Mode:           "safe",
		ConflictPolicy: "rename",
		Status:         RecoveryJobQueued,
		CreatedAt:      base,
	}); err != nil {
		t.Fatalf("CreateRecoveryJob: %v", err)
	}

	stats, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats: %v", err)
	}
	if len(stats.Resources) != 4 {
		t.Fatalf("len(resources) = %d, want 4", len(stats.Resources))
	}

	rowsByResource := make(map[string]int64, len(stats.Resources))
	for _, item := range stats.Resources {
		rowsByResource[item.Resource] = item.Rows
	}
	for _, resource := range []string{
		StorageResourceTimeline,
		StorageResourceActivityLog,
		StorageResourceGuardrailLog,
		StorageResourceRecoveryLog,
	} {
		if rowsByResource[resource] < 1 {
			t.Fatalf("resource %q rows = %d, want >= 1", resource, rowsByResource[resource])
		}
	}

	results, err := s.FlushStorageResource(ctx, StorageResourceAll)
	if err != nil {
		t.Fatalf("FlushStorageResource(all): %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}

	after, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats after flush: %v", err)
	}
	for _, item := range after.Resources {
		if item.Rows != 0 {
			t.Fatalf("resource %q rows after flush = %d, want 0", item.Resource, item.Rows)
		}
	}
}

func TestFlushStorageRejectsInvalidResource(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.FlushStorageResource(context.Background(), "unknown")
	if !errors.Is(err, ErrInvalidStorageResource) {
		t.Fatalf("error = %v, want ErrInvalidStorageResource", err)
	}
}
