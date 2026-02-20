package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSchedulerCRUD(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Insert a schedule.
	sched, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		RunbookID:    "runbook-1",
		Name:         "Daily backup",
		ScheduleType: "cron",
		CronExpr:     "0 2 * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}
	if sched.ID == "" {
		t.Fatal("schedule ID is empty")
	}
	if sched.Name != "Daily backup" {
		t.Fatalf("name = %q, want %q", sched.Name, "Daily backup")
	}
	if !sched.Enabled {
		t.Fatal("enabled = false, want true")
	}

	// List all schedules.
	all, err := s.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len = %d, want 1", len(all))
	}

	// Update the schedule.
	updated, err := s.UpdateOpsSchedule(ctx, OpsScheduleWrite{
		ID:           sched.ID,
		RunbookID:    "runbook-1",
		Name:         "Weekly backup",
		ScheduleType: "cron",
		CronExpr:     "0 2 * * 0",
		Timezone:     "UTC",
		Enabled:      false,
		NextRunAt:    "",
	})
	if err != nil {
		t.Fatalf("UpdateOpsSchedule: %v", err)
	}
	if updated.Name != "Weekly backup" {
		t.Fatalf("updated name = %q, want %q", updated.Name, "Weekly backup")
	}
	if updated.Enabled {
		t.Fatal("enabled = true, want false")
	}

	// Delete the schedule.
	if err := s.DeleteOpsSchedule(ctx, sched.ID); err != nil {
		t.Fatalf("DeleteOpsSchedule: %v", err)
	}

	all, err = s.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("len = %d, want 0 after delete", len(all))
	}
}

func TestSchedulerUpdateNonexistent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.UpdateOpsSchedule(context.Background(), OpsScheduleWrite{
		ID:           "nonexistent",
		RunbookID:    "runbook-1",
		Name:         "Test",
		ScheduleType: "cron",
	})
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestSchedulerDeleteNonexistent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	err := s.DeleteOpsSchedule(context.Background(), "nonexistent")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestListDueSchedules(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// Insert a due schedule (next_run_at in the past).
	_, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		RunbookID:    "rb-1",
		Name:         "Due schedule",
		ScheduleType: "cron",
		CronExpr:     "* * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    past.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule (due): %v", err)
	}

	// Insert a not-due schedule (next_run_at in the future).
	_, err = s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		RunbookID:    "rb-2",
		Name:         "Future schedule",
		ScheduleType: "cron",
		CronExpr:     "0 0 * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    future.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule (future): %v", err)
	}

	due, err := s.ListDueSchedules(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueSchedules: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("len(due) = %d, want 1", len(due))
	}
	if due[0].Name != "Due schedule" {
		t.Fatalf("due schedule name = %q, want %q", due[0].Name, "Due schedule")
	}
}

func TestListDueSchedulesNoLimit(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)

	for i := range 3 {
		_, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
			RunbookID:    "rb-1",
			Name:         "sched",
			ScheduleType: "cron",
			CronExpr:     "* * * * *",
			Timezone:     "UTC",
			Enabled:      true,
			NextRunAt:    past.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("InsertOpsSchedule: %v", err)
		}
	}

	// limit=0 means no limit.
	due, err := s.ListDueSchedules(ctx, now, 0)
	if err != nil {
		t.Fatalf("ListDueSchedules: %v", err)
	}
	if len(due) != 3 {
		t.Fatalf("len(due) = %d, want 3", len(due))
	}
}

func TestListSchedulesByRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	for range 2 {
		_, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
			RunbookID:    "rb-target",
			Name:         "target-sched",
			ScheduleType: "cron",
			CronExpr:     "* * * * *",
			Timezone:     "UTC",
			Enabled:      true,
		})
		if err != nil {
			t.Fatalf("InsertOpsSchedule: %v", err)
		}
	}
	_, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		RunbookID:    "rb-other",
		Name:         "other-sched",
		ScheduleType: "once",
		Timezone:     "UTC",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	schedules, err := s.ListSchedulesByRunbook(ctx, "rb-target")
	if err != nil {
		t.Fatalf("ListSchedulesByRunbook: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("len = %d, want 2", len(schedules))
	}
}

func TestUpdateScheduleAfterRun(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	sched, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		RunbookID:    "rb-1",
		Name:         "test",
		ScheduleType: "cron",
		CronExpr:     "* * * * *",
		Timezone:     "UTC",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	next := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	if err := s.UpdateScheduleAfterRun(ctx, sched.ID, now, "success", next, true); err != nil {
		t.Fatalf("UpdateScheduleAfterRun: %v", err)
	}

	all, err := s.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len = %d, want 1", len(all))
	}
	if all[0].LastRunStatus != "success" {
		t.Fatalf("LastRunStatus = %q, want success", all[0].LastRunStatus)
	}
}

func TestDeleteSchedulesByRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	for range 3 {
		_, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
			RunbookID:    "rb-delete-me",
			Name:         "sched",
			ScheduleType: "cron",
			CronExpr:     "* * * * *",
			Timezone:     "UTC",
			Enabled:      true,
		})
		if err != nil {
			t.Fatalf("InsertOpsSchedule: %v", err)
		}
	}

	if err := s.DeleteSchedulesByRunbook(ctx, "rb-delete-me"); err != nil {
		t.Fatalf("DeleteSchedulesByRunbook: %v", err)
	}

	all, err := s.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("len = %d, want 0 after bulk delete", len(all))
	}
}

func TestInsertOpsScheduleWithExplicitID(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	sched, err := s.InsertOpsSchedule(ctx, OpsScheduleWrite{
		ID:           "custom-id-123",
		RunbookID:    "rb-1",
		Name:         "Custom ID",
		ScheduleType: "once",
		Timezone:     "UTC",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}
	if sched.ID != "custom-id-123" {
		t.Fatalf("id = %q, want %q", sched.ID, "custom-id-123")
	}
}
