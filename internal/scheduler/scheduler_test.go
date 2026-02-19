package scheduler

import (
	"context"
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

func TestTick_DueScheduleCreatesRun(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	hub := events.NewHub()
	svc := New(st, st, Options{EventHub: hub})

	ctx := context.Background()

	// Create a runbook with 0 steps (completes instantly).
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:    "tick-test",
		Enabled: true,
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

	// Wait for the async goroutine to complete so the store can close cleanly.
	time.Sleep(300 * time.Millisecond)
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

	time.Sleep(300 * time.Millisecond)
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

	// Wait for async run to complete (0 steps = instant).
	time.Sleep(500 * time.Millisecond)

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
	time.Sleep(500 * time.Millisecond)

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

func TestStartStop(t *testing.T) {
	t.Parallel()
	st := testStore(t)
	svc := New(st, st, Options{TickInterval: 100 * time.Millisecond})

	ctx := context.Background()

	svc.Start(ctx)

	// Let it tick a few times.
	time.Sleep(250 * time.Millisecond)

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
	}
	if got.Enabled {
		t.Fatal("orphan schedule should be disabled after auto-heal")
	}
	if got.NextRunAt != "" {
		t.Fatalf("orphan schedule next_run_at should be empty, got %q", got.NextRunAt)
	}
}
