package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestNew_DefaultTickInterval(t *testing.T) {
	t.Parallel()
	st := testStore(t)

	svc := New(st, st, Options{})
	if svc.opts.TickInterval != defaultTickInterval {
		t.Fatalf("expected %v, got %v", defaultTickInterval, svc.opts.TickInterval)
	}
}

func TestNew_CustomTickInterval(t *testing.T) {
	t.Parallel()
	st := testStore(t)

	svc := New(st, st, Options{TickInterval: 10 * time.Second})
	if svc.opts.TickInterval != 10*time.Second {
		t.Fatalf("expected 10s, got %v", svc.opts.TickInterval)
	}
}

func TestNew_NegativeTickInterval(t *testing.T) {
	t.Parallel()
	st := testStore(t)

	svc := New(st, st, Options{TickInterval: -1 * time.Second})
	if svc.opts.TickInterval != defaultTickInterval {
		t.Fatalf("expected default %v, got %v", defaultTickInterval, svc.opts.TickInterval)
	}
}

func TestComputeNextRun_CronAdvances(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	sched := store.OpsSchedule{
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *", // every 5 minutes
		Timezone:     "UTC",
	}

	nextRun, enabled := svc.computeNextRun(sched)
	if !enabled {
		t.Fatal("expected enabled=true for cron schedule")
	}
	if nextRun == "" {
		t.Fatal("expected non-empty nextRun for cron schedule")
	}

	parsed, err := time.Parse(time.RFC3339, nextRun)
	if err != nil {
		t.Fatalf("nextRun is not valid RFC3339: %v", err)
	}
	if !parsed.After(time.Now().UTC()) {
		t.Fatalf("nextRun should be in the future, got %v", parsed)
	}
}

func TestComputeNextRun_OnceDisables(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	sched := store.OpsSchedule{
		ScheduleType: "once",
	}

	nextRun, enabled := svc.computeNextRun(sched)
	if enabled {
		t.Fatal("expected enabled=false for once schedule")
	}
	if nextRun != "" {
		t.Fatalf("expected empty nextRun for once schedule, got %q", nextRun)
	}
}

func TestComputeNextRun_InvalidCron(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	sched := store.OpsSchedule{
		ScheduleType: "cron",
		CronExpr:     "not-a-cron",
		Timezone:     "UTC",
	}

	nextRun, enabled := svc.computeNextRun(sched)
	if enabled {
		t.Fatal("expected enabled=false for invalid cron")
	}
	if nextRun != "" {
		t.Fatalf("expected empty nextRun for invalid cron, got %q", nextRun)
	}
}

func TestComputeNextRun_InvalidTimezone(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	sched := store.OpsSchedule{
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "Invalid/Zone",
	}

	// Should fall back to UTC and still produce a valid next run.
	nextRun, enabled := svc.computeNextRun(sched)
	if !enabled {
		t.Fatal("expected enabled=true even with invalid timezone (falls back to UTC)")
	}
	if nextRun == "" {
		t.Fatal("expected non-empty nextRun with UTC fallback")
	}
}

// neverFiringCron parses cleanly but has no occurrence: February never has a
// 30th, so cron.Schedule.Next gives up after five years and returns zero time.
const neverFiringCron = "0 0 30 2 *"

func TestComputeNextRun_NeverFiringCron(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	sched := store.OpsSchedule{
		ScheduleType: "cron",
		CronExpr:     neverFiringCron,
		Timezone:     "UTC",
	}

	nextRun, enabled := svc.computeNextRun(sched)
	if enabled {
		t.Fatal("expected enabled=false for a cron expression with no next run")
	}
	if nextRun != "" {
		t.Fatalf("expected empty nextRun, got %q", nextRun)
	}
}

func TestTick_NeverFiringCronScheduleDisabled(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{Name: "never-fires", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// This is what the zero time from Next persists as: always <= now, so the
	// schedule is due on every tick and beyond the catch-up window.
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "never-fires-schedule",
		ScheduleType: "cron",
		CronExpr:     neverFiringCron,
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    time.Time{}.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.tick(ctx)
	svc.wg.Wait()

	got := findSchedule(t, st, sched.ID)
	if got.Enabled {
		t.Fatal("a schedule whose cron never fires should be disabled, not left due forever")
	}
	if got.NextRunAt != "" {
		t.Fatalf("next_run_at should be cleared, got %q", got.NextRunAt)
	}
}

func TestRecomputeNextRun_InvalidCronDisables(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{Name: "bad-cron", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-48 * time.Hour)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "bad-cron-schedule",
		ScheduleType: "cron",
		CronExpr:     "not-a-cron",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    stale.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.recomputeNextRun(ctx, sched)

	got := findSchedule(t, st, sched.ID)
	if got.Enabled {
		t.Fatal("a schedule with an unparseable cron should be disabled, not left due forever")
	}
	if got.NextRunAt != "" {
		t.Fatalf("next_run_at should be cleared, got %q", got.NextRunAt)
	}
}

func TestExecuteDueSchedule_DisablesOnUnmetRequiredParameters(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "unmet-params",
		Enabled: true,
		Parameters: []store.RunbookParameter{
			{Name: "TARGET", Label: "Target", Type: "string", Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-1 * time.Minute)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "unmet-params-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.tick(ctx)
	svc.wg.Wait()

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no run for a schedule with unmet required parameters, got %d", len(runs))
	}

	got := findSchedule(t, st, sched.ID)
	if got.Enabled {
		t.Fatal("a schedule the scheduler can never satisfy should be disabled, not retried every tick")
	}
	if got.NextRunAt != "" {
		t.Fatalf("next_run_at should be cleared, got %q", got.NextRunAt)
	}
}

func findSchedule(t *testing.T, st *store.Store, id string) store.OpsSchedule {
	t.Helper()
	schedules, err := st.ListOpsSchedules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("schedule %q not found", id)
	return store.OpsSchedule{}
}

func TestTick_NoDueSchedules(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	// No schedules exist; tick should not panic.
	svc.tick(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}

type failingScheduleUpdateRepo struct {
	updateCalls int
}

func (r *failingScheduleUpdateRepo) ListDueSchedules(context.Context, time.Time, int) ([]store.OpsSchedule, error) {
	return nil, nil
}

func (r *failingScheduleUpdateRepo) GetOpsRunbook(_ context.Context, id string) (store.OpsRunbook, error) {
	return store.OpsRunbook{ID: id, Name: "runbook"}, nil
}

func (r *failingScheduleUpdateRepo) CreateOpsRunbookRun(context.Context, store.OpsRunbookRunWrite) (store.OpsRunbookRun, error) {
	return store.OpsRunbookRun{}, nil
}

func (r *failingScheduleUpdateRepo) UpdateScheduleAfterRun(context.Context, string, string, string, string, bool) error {
	r.updateCalls++
	return errors.New("update failed")
}

func (r *failingScheduleUpdateRepo) UpdateScheduleLastRun(context.Context, string, string, string) error {
	r.updateCalls++
	return errors.New("update failed")
}

type schedulerRunbookRepo struct{}

func (schedulerRunbookRepo) UpdateOpsRunbookRun(_ context.Context, update store.OpsRunbookRunUpdate) (store.OpsRunbookRun, error) {
	return store.OpsRunbookRun{
		ID:        update.RunID,
		RunbookID: "runbook-1",
		Status:    update.Status,
	}, nil
}

func (schedulerRunbookRepo) GetOpsRunbook(_ context.Context, id string) (store.OpsRunbook, error) {
	return store.OpsRunbook{ID: id, Name: "runbook", Enabled: true}, nil
}

func (schedulerRunbookRepo) GetOpsRunbookRun(_ context.Context, id string) (store.OpsRunbookRun, error) {
	return store.OpsRunbookRun{ID: id, RunbookID: "runbook-1", Status: "succeeded"}, nil
}

func TestExecuteRunbookHandlesScheduleUpdateError(t *testing.T) {
	t.Parallel()

	repo := &failingScheduleUpdateRepo{}
	svc := New(repo, schedulerRunbookRepo{}, Options{})

	svc.executeRunbook(context.Background(), store.OpsRunbookRun{
		ID:        "job-1",
		RunbookID: "runbook-1",
		Definition: &store.OpsRunbookExecutionSnapshot{
			SchemaVersion: 1,
			RunbookID:     "runbook-1",
		},
	}, "schedule-1")

	if repo.updateCalls != 1 {
		t.Fatalf("UpdateScheduleLastRun calls = %d, want 1", repo.updateCalls)
	}
}

type busyTargetScheduleRepo struct {
	advancedStatus string
	nextRunAt      string
	lastStatus     string
}

func (r *busyTargetScheduleRepo) ListDueSchedules(context.Context, time.Time, int) ([]store.OpsSchedule, error) {
	return nil, nil
}

func (r *busyTargetScheduleRepo) GetOpsRunbook(_ context.Context, id string) (store.OpsRunbook, error) {
	return store.OpsRunbook{
		ID: id, Name: "Busy", TargetService: "nginx",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "Run", Command: "true"}},
	}, nil
}

func (r *busyTargetScheduleRepo) CreateOpsRunbookRun(context.Context, store.OpsRunbookRunWrite) (store.OpsRunbookRun, error) {
	return store.OpsRunbookRun{}, store.ErrOpsRunbookTargetBusy
}

func (r *busyTargetScheduleRepo) UpdateScheduleAfterRun(_ context.Context, _ string, _, status, nextRunAt string, _ bool) error {
	r.advancedStatus = status
	r.nextRunAt = nextRunAt
	return nil
}

func (r *busyTargetScheduleRepo) UpdateScheduleLastRun(_ context.Context, _, _, status string) error {
	r.lastStatus = status
	return nil
}

func TestDueScheduleRecordsTargetBusyAfterAdvancingNextRun(t *testing.T) {
	t.Parallel()

	repo := &busyTargetScheduleRepo{}
	svc := New(repo, schedulerRunbookRepo{}, Options{})
	svc.executeDueSchedule(context.Background(), store.OpsSchedule{
		ID:           "schedule-1",
		RunbookID:    "runbook-1",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
	}, time.Now().UTC())

	if repo.advancedStatus != "running" || repo.nextRunAt == "" {
		t.Fatalf("advanced schedule = (%q, %q)", repo.advancedStatus, repo.nextRunAt)
	}
	if repo.lastStatus != "target_busy" {
		t.Fatalf("last status = %q, want target_busy", repo.lastStatus)
	}
}

func TestTick_DueScheduleCreatesRun(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	hub := events.NewHub()
	svc := New(st, st, Options{EventHub: hub})

	ctx := context.Background()

	// Create a runbook with 0 steps (completes instantly).
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:          "tick-test",
		Enabled:       true,
		TargetService: "nginx",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a schedule that is already due.
	past := time.Now().UTC().Add(-1 * time.Minute)
	_, err = st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "due-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	// tick creates the run synchronously; the goroutine executes it async.
	svc.tick(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one run after tick")
	}
	if runs[0].RunbookID != rb.ID {
		t.Fatalf("run runbook ID = %q, want %q", runs[0].RunbookID, rb.ID)
	}
	if runs[0].Source != store.OpsRunbookRunSourceScheduler ||
		runs[0].TargetKind != store.OpsRunbookRunTargetService ||
		runs[0].TargetName != "nginx" {
		t.Fatalf("run context = (%q, %q, %q)", runs[0].Source, runs[0].TargetKind, runs[0].TargetName)
	}

	// Wait for the async goroutine to complete so the store can close cleanly.
	svc.wg.Wait()
}

func TestTick_SkipsScheduleAlreadyInFlight(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{EventHub: events.NewHub()})
	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{Name: "overlap-test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-1 * time.Minute)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "overlap-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a run still in flight for this schedule (e.g. cron interval
	// shorter than the run). A tick must not create a second, overlapping run.
	if !svc.claimSchedule(sched.ID) {
		t.Fatal("first claim should succeed")
	}
	svc.tick(ctx)
	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no run while the schedule is in flight, got %d", len(runs))
	}

	// After the in-flight run finishes, the next tick may trigger again.
	svc.releaseSchedule(sched.ID)
	svc.tick(ctx)
	runs, err = st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected exactly one run after release, got %d", len(runs))
	}

	svc.wg.Wait()
}

func TestTick_FutureScheduleNotTriggered(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "future-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	future := time.Now().UTC().Add(1 * time.Hour)
	_, err = st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "future-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    future.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.tick(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs for future schedule, got %d", len(runs))
	}
}

func TestCatchUpMissedRuns_WithinWindow(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	hub := events.NewHub()
	svc := New(st, st, Options{EventHub: hub})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "catchup-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Missed by 2 hours (within the 24h catchUpWindow).
	missed := time.Now().UTC().Add(-2 * time.Hour)
	_, err = st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "missed-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    missed.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.catchUpMissedRuns(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("expected a catch-up run within the 24h window")
	}

	svc.wg.Wait()
}

func TestCatchUpMissedRuns_BeyondWindow(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "old-schedule-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Missed by 48 hours (beyond the 24h window).
	old := time.Now().UTC().Add(-48 * time.Hour)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "old-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    old.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.catchUpMissedRuns(ctx)

	// Should NOT create a run (too old); instead, it recomputes nextRunAt.
	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs for schedule beyond window, got %d", len(runs))
	}

	// Verify the schedule's nextRunAt was recomputed to a future time.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == sched.ID {
			if s.NextRunAt == "" {
				t.Fatal("expected nextRunAt to be recomputed, got empty")
			}
			parsed, parseErr := time.Parse(time.RFC3339, s.NextRunAt)
			if parseErr != nil {
				t.Fatalf("nextRunAt not valid RFC3339: %v", parseErr)
			}
			if !parsed.After(time.Now().UTC()) {
				t.Fatalf("recomputed nextRunAt should be in the future, got %v", parsed)
			}
			return
		}
	}
	t.Fatal("schedule not found after recompute")
}

func TestCatchUpMissedRuns_DisabledScheduleSkipped(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "disabled-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	missed := time.Now().UTC().Add(-1 * time.Hour)
	_, err = st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "disabled-schedule",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      false, // disabled
		NextRunAt:    missed.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.catchUpMissedRuns(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs for disabled schedule, got %d", len(runs))
	}
}

func TestCatchUpMissedRuns_OnceScheduleBeyondWindowDisabled(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "once-old-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// One-time schedule that's far past due.
	old := time.Now().UTC().Add(-48 * time.Hour)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "once-old",
		ScheduleType: "once",
		RunAt:        old.Format(time.RFC3339),
		Enabled:      true,
		NextRunAt:    old.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.catchUpMissedRuns(ctx)

	// Once schedule beyond window should be disabled.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == sched.ID {
			if s.Enabled {
				t.Fatal("expected once schedule beyond window to be disabled")
			}
			return
		}
	}
	t.Fatal("schedule not found")
}

func TestCronRecurrence_AfterRunCompletion(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	hub := events.NewHub()
	svc := New(st, st, Options{EventHub: hub})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "cron-recurrence-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Schedule already due.
	past := time.Now().UTC().Add(-1 * time.Minute)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "recurring-cron",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Tick triggers the run.
	svc.tick(ctx)

	// Wait for the async run to complete, including its OnFinish store write.
	svc.wg.Wait()

	// Verify: schedule must still be enabled AND have a valid future next_run_at.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == sched.ID {
			found = &schedules[i]
			break
		}
	}
	if found == nil {
		t.Fatal("schedule not found after run")
		return
	}
	if !found.Enabled {
		t.Fatal("cron schedule should remain enabled after run")
	}
	if found.NextRunAt == "" {
		t.Fatal("cron schedule next_run_at should not be empty after run")
	}
	parsed, parseErr := time.Parse(time.RFC3339, found.NextRunAt)
	if parseErr != nil {
		t.Fatalf("next_run_at is not valid RFC3339: %v", parseErr)
	}
	if !parsed.After(time.Now().UTC().Add(-1 * time.Minute)) {
		t.Fatalf("next_run_at should be recent/future, got %v", parsed)
	}
	if found.LastRunStatus == "running" {
		t.Fatal("last_run_status should be terminal (not 'running') after completion")
	}
}

func TestOnceSchedule_DisabledAfterRunCompletion(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	hub := events.NewHub()
	svc := New(st, st, Options{EventHub: hub})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "once-completion-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().Add(-1 * time.Minute)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "one-time",
		ScheduleType: "once",
		RunAt:        past.Format(time.RFC3339),
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.tick(ctx)
	svc.wg.Wait()

	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == sched.ID {
			if s.Enabled {
				t.Fatal("once schedule should be disabled after run completion")
			}
			if s.NextRunAt != "" {
				t.Fatalf("once schedule next_run_at should be empty, got %q", s.NextRunAt)
			}
			return
		}
	}
	t.Fatal("schedule not found")
}

func TestTick_StaleScheduleRecomputed(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{})

	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "stale-tick-test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Schedule due 48h ago — beyond catchUpWindow.
	stale := time.Now().UTC().Add(-48 * time.Hour)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "stale-cron",
		ScheduleType: "cron",
		CronExpr:     "*/5 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    stale.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	// tick should recompute, not execute.
	svc.tick(ctx)

	runs, err := st.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs for stale schedule in tick, got %d", len(runs))
	}

	// Verify next_run_at was recomputed to the future.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == sched.ID {
			parsed, parseErr := time.Parse(time.RFC3339, s.NextRunAt)
			if parseErr != nil {
				t.Fatalf("recomputed next_run_at not valid: %v", parseErr)
			}
			if !parsed.After(time.Now().UTC()) {
				t.Fatalf("recomputed next_run_at should be in the future, got %v", parsed)
			}
			return
		}
	}
	t.Fatal("schedule not found")
}

func TestPublish_NilHub(t *testing.T) {
	t.Parallel()
	st := testStore(t)

	svc := New(st, st, Options{EventHub: nil})
	// Should not panic.
	svc.publish("test.event", map[string]any{"key": "value"})
}

func TestPublish_NilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	// Should not panic.
	svc.publish("test.event", map[string]any{"key": "value"})
}

// listCountingRepo signals every ListDueSchedules call so a test can wait for
// the tick loop to actually run instead of sleeping for a guessed duration.
type listCountingRepo struct {
	calls chan struct{}
}

func (r *listCountingRepo) ListDueSchedules(context.Context, time.Time, int) ([]store.OpsSchedule, error) {
	select {
	case r.calls <- struct{}{}:
	default:
	}
	return nil, nil
}

func (r *listCountingRepo) GetOpsRunbook(_ context.Context, id string) (store.OpsRunbook, error) {
	return store.OpsRunbook{ID: id}, nil
}

func (r *listCountingRepo) CreateOpsRunbookRun(context.Context, store.OpsRunbookRunWrite) (store.OpsRunbookRun, error) {
	return store.OpsRunbookRun{}, nil
}

func (r *listCountingRepo) UpdateScheduleAfterRun(context.Context, string, string, string, string, bool) error {
	return nil
}

func (r *listCountingRepo) UpdateScheduleLastRun(context.Context, string, string, string) error {
	return nil
}

func TestStartStop(t *testing.T) {
	t.Parallel()
	repo := &listCountingRepo{calls: make(chan struct{}, 4)}
	svc := New(repo, schedulerRunbookRepo{}, Options{TickInterval: 10 * time.Millisecond})

	ctx := context.Background()

	svc.Start(ctx)

	// Wait for the catch-up pass plus at least one tick, so Stop is exercised
	// against a loop that is genuinely running.
	for i := range 2 {
		select {
		case <-repo.calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for scheduler pass %d", i+1)
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	svc.Stop(stopCtx)

	// Should not panic on double stop.
	svc.Stop(stopCtx)
}

func TestStart_NilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	ctx := context.Background()

	// Should not panic.
	svc.Start(ctx)
	svc.Stop(ctx)
}

func TestExecuteDueSchedule_AutoHealsOrphanRunbook(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	ctx := context.Background()
	hub := events.NewHub()

	svc := New(st, st, Options{TickInterval: time.Hour, EventHub: hub})

	// Create a schedule pointing to a nonexistent runbook.
	due := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    "nonexistent-runbook-id",
		Name:         "orphan-schedule",
		ScheduleType: "cron",
		CronExpr:     "0 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    due,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	// Execute tick — should auto-heal the orphan.
	svc.tick(ctx)

	// Verify the schedule was disabled.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	var got *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == sched.ID {
			got = &schedules[i]
			break
		}
	}
	if got == nil {
		t.Fatal("schedule not found")
		return
	}
	if got.Enabled {
		t.Fatal("orphan schedule should be disabled after auto-heal")
	}
	if got.NextRunAt != "" {
		t.Fatalf("orphan schedule next_run_at should be empty, got %q", got.NextRunAt)
	}
}
