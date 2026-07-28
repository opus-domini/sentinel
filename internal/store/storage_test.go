package store

import (
	"context"
	"database/sql"
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
	seedStorageStatsData(ctx, t, s, base)

	stats, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats: %v", err)
	}
	if len(stats.Resources) != 2 {
		t.Fatalf("len(resources) = %d, want 2", len(stats.Resources))
	}

	statsByResource := make(map[string]StorageResourceStat, len(stats.Resources))
	for _, item := range stats.Resources {
		statsByResource[item.Resource] = item
	}
	activity := statsByResource[StorageResourceActivityLog]
	if activity.TotalRows != 1 || activity.FlushableRows != 1 || activity.ProtectedRows != 0 {
		t.Fatalf("activity stats = %+v, want total=1 flushable=1 protected=0", activity)
	}
	jobs := statsByResource[StorageResourceOpsJobs]
	if jobs.TotalRows != 1 || jobs.FlushableRows != 0 || jobs.ProtectedRows != 1 {
		t.Fatalf("job stats = %+v, want total=1 flushable=0 protected=1", jobs)
	}

	results, err := s.FlushStorageResource(ctx, StorageResourceAll)
	if err != nil {
		t.Fatalf("FlushStorageResource(all): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].RemovedRows != 1 {
		t.Fatalf("activity flush result = %+v, want one removed row", results[0])
	}
	if results[1].RemovedRows != 0 || results[1].ProtectedRows != 1 {
		t.Fatalf("job flush result = %+v, want removed=0 protected=1", results[1])
	}

	after, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats after flush: %v", err)
	}
	for _, item := range after.Resources {
		switch item.Resource {
		case StorageResourceActivityLog:
			if item.TotalRows != 0 {
				t.Fatalf("activity rows after flush = %d, want 0", item.TotalRows)
			}
		case StorageResourceOpsJobs:
			if item.TotalRows != 1 || item.ProtectedRows != 1 {
				t.Fatalf("job stats after flush = %+v, want active row preserved", item)
			}
		}
	}
}

func TestFlushOpsJobsPreservesActiveStates(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	runIDs := seedStorageJobs(ctx, t, s)

	stats, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats: %v", err)
	}
	jobs := storageStat(t, stats, StorageResourceOpsJobs)
	if jobs.TotalRows != 5 || jobs.FlushableRows != 2 || jobs.ProtectedRows != 3 {
		t.Fatalf("job stats = %+v, want total=5 flushable=2 protected=3", jobs)
	}

	results, err := s.FlushStorageResource(ctx, StorageResourceOpsJobs)
	if err != nil {
		t.Fatalf("FlushStorageResource(ops-jobs): %v", err)
	}
	if len(results) != 1 || results[0].RemovedRows != 2 || results[0].ProtectedRows != 3 {
		t.Fatalf("flush results = %+v, want removed=2 protected=3", results)
	}

	for _, status := range []string{
		OpsRunbookStatusQueued,
		OpsRunbookStatusRunning,
		OpsRunbookStatusWaitingApproval,
	} {
		if _, err := s.GetOpsRunbookRun(ctx, runIDs[status]); err != nil {
			t.Fatalf("protected status %q was removed: %v", status, err)
		}
	}
	for _, status := range []string{
		OpsRunbookStatusSucceeded,
		OpsRunbookStatusFailed,
	} {
		if _, err := s.GetOpsRunbookRun(ctx, runIDs[status]); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("terminal status %q lookup error = %v, want sql.ErrNoRows", status, err)
		}
	}
}

func TestFlushAllRollsBackWhenSecondResourceFails(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	seedStorageStatsData(ctx, t, s, base)
	if _, err := s.db.ExecContext(ctx, `UPDATE ops_runbook_runs SET status = ?`,
		OpsRunbookStatusSucceeded,
	); err != nil {
		t.Fatalf("mark job terminal: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_ops_jobs_flush
		BEFORE DELETE ON ops_runbook_runs
		BEGIN
			SELECT RAISE(ABORT, 'injected ops jobs failure');
		END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if _, err := s.FlushStorageResource(ctx, StorageResourceAll); err == nil {
		t.Fatal("FlushStorageResource(all) error = nil, want injected failure")
	}

	stats, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats after rollback: %v", err)
	}
	if activity := storageStat(t, stats, StorageResourceActivityLog); activity.TotalRows != 1 {
		t.Fatalf("activity stats after rollback = %+v, want original row", activity)
	}
	if jobs := storageStat(t, stats, StorageResourceOpsJobs); jobs.TotalRows != 1 {
		t.Fatalf("job stats after rollback = %+v, want original row", jobs)
	}
}

func seedStorageStatsData(ctx context.Context, t *testing.T, s *Store, base time.Time) {
	t.Helper()
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
	runbook, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID: "storage.stats", Name: "Storage Stats", Steps: []OpsRunbookStep{{Type: "run", Title: "Run", Command: "true"}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	if _, err := s.CreateOpsRunbookRun(ctx, testRunWrite(t, s, runbook.ID, base, nil)); err != nil {
		t.Fatalf("CreateOpsRunbookRun: %v", err)
	}
}

func seedStorageJobs(
	ctx context.Context,
	t *testing.T,
	s *Store,
) map[string]string {
	t.Helper()
	runbook, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "storage.jobs",
		Name: "Storage Jobs",
		Steps: []OpsRunbookStep{{
			Type: "run", Title: "Run", Command: "true",
		}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	statuses := []string{
		OpsRunbookStatusQueued,
		OpsRunbookStatusRunning,
		OpsRunbookStatusWaitingApproval,
		OpsRunbookStatusSucceeded,
		OpsRunbookStatusFailed,
	}
	runIDs := make(map[string]string, len(statuses))
	base := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	for idx, status := range statuses {
		run, createErr := s.CreateOpsRunbookRun(
			ctx,
			testRunWrite(t, s, runbook.ID, base.Add(time.Duration(idx)*time.Minute), nil),
		)
		if createErr != nil {
			t.Fatalf("CreateOpsRunbookRun(%s): %v", status, createErr)
		}
		if status != OpsRunbookStatusQueued {
			run, createErr = s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
				RunID:  run.ID,
				Status: status,
			})
			if createErr != nil {
				t.Fatalf("UpdateOpsRunbookRun(%s): %v", status, createErr)
			}
		}
		runIDs[status] = run.ID
	}
	return runIDs
}

func storageStat(t *testing.T, stats StorageStats, resource string) StorageResourceStat {
	t.Helper()
	for _, item := range stats.Resources {
		if item.Resource == resource {
			return item
		}
	}
	t.Fatalf("storage resource %q not found in %+v", resource, stats.Resources)
	return StorageResourceStat{}
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
