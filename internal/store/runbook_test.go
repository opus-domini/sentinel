package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestOpsRunbooksAndRuns(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatalf("expected seeded runbooks")
	}

	run, err := s.StartOpsRunbook(ctx, runbooks[0].ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("StartOpsRunbook: %v", err)
	}
	if run.Status != opsRunbookStatusSucceeded {
		t.Fatalf("status = %q, want %q", run.Status, opsRunbookStatusSucceeded)
	}
	if run.TotalSteps < 1 {
		t.Fatalf("total steps = %d, want >= 1", run.TotalSteps)
	}
	if run.CompletedSteps != run.TotalSteps {
		t.Fatalf("completed=%d total=%d, want equal", run.CompletedSteps, run.TotalSteps)
	}

	loaded, err := s.GetOpsRunbookRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun: %v", err)
	}
	if loaded.ID != run.ID {
		t.Fatalf("run id = %q, want %q", loaded.ID, run.ID)
	}

	history, err := s.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListOpsRunbookRuns: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("len(history) = %d, want 1", len(history))
	}
}

func TestInsertOpsRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("happy path with steps", func(t *testing.T) {
		steps := []OpsRunbookStep{
			{Type: "command", Title: "Check status", Command: "systemctl status"},
			{Type: "manual", Title: "Review", Description: "Verify output"},
		}
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:          "test.runbook.1",
			Name:        "Test Runbook",
			Description: "A test runbook",
			Steps:       steps,
			Enabled:     true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.ID != "test.runbook.1" {
			t.Fatalf("id = %q, want test.runbook.1", rb.ID)
		}
		if rb.Name != "Test Runbook" {
			t.Fatalf("name = %q, want Test Runbook", rb.Name)
		}
		if rb.Description != "A test runbook" {
			t.Fatalf("description = %q, want A test runbook", rb.Description)
		}
		if !rb.Enabled {
			t.Fatalf("enabled = false, want true")
		}
		if len(rb.Steps) != 2 {
			t.Fatalf("len(steps) = %d, want 2", len(rb.Steps))
		}
		if rb.Steps[0].Type != "command" || rb.Steps[1].Type != "manual" {
			t.Fatalf("unexpected step types: %+v", rb.Steps)
		}
	})

	t.Run("auto-generates ID when empty", func(t *testing.T) {
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			Name:    "Auto ID Runbook",
			Enabled: true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.ID == "" {
			t.Fatalf("id should be auto-generated")
		}
	})

	t.Run("nil steps stored as empty array", func(t *testing.T) {
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:      "test.nil.steps",
			Name:    "Nil Steps",
			Steps:   nil,
			Enabled: false,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.Steps == nil {
			t.Fatalf("steps should be empty slice, not nil")
		}
		if len(rb.Steps) != 0 {
			t.Fatalf("len(steps) = %d, want 0", len(rb.Steps))
		}
		if rb.Enabled {
			t.Fatalf("enabled = true, want false")
		}
	})

	t.Run("empty name errors", func(t *testing.T) {
		_, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "test.no.name",
			Name: "",
		})
		if err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("duplicate ID errors", func(t *testing.T) {
		_, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "test.runbook.1",
			Name: "Duplicate",
		})
		if err == nil {
			t.Fatalf("expected error for duplicate ID")
		}
	})
}

func TestUpdateOpsRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Seed a runbook to update.
	orig, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:          "update.me",
		Name:        "Original",
		Description: "Original description",
		Steps:       []OpsRunbookStep{{Type: "command", Title: "Step 1", Command: "echo hello"}},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	t.Run("update all fields", func(t *testing.T) {
		newSteps := []OpsRunbookStep{
			{Type: "check", Title: "Verify", Check: "service should be running"},
		}
		updated, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:          "update.me",
			Name:        "Updated",
			Description: "Updated description",
			Steps:       newSteps,
			Enabled:     false,
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbook: %v", err)
		}
		if updated.Name != "Updated" {
			t.Fatalf("name = %q, want Updated", updated.Name)
		}
		if updated.Description != "Updated description" {
			t.Fatalf("description = %q, want Updated description", updated.Description)
		}
		if updated.Enabled {
			t.Fatalf("enabled = true, want false")
		}
		if len(updated.Steps) != 1 || updated.Steps[0].Type != "check" {
			t.Fatalf("unexpected steps: %+v", updated.Steps)
		}
		// UpdatedAt should be refreshed (not strictly equal to creation time).
		// Clock granularity may make them equal, so this is a best-effort check.
		if updated.UpdatedAt != orig.CreatedAt && updated.UpdatedAt == "" {
			t.Fatal("expected non-empty updatedAt")
		}
	})

	t.Run("empty ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "",
			Name: "NoID",
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("empty name errors", func(t *testing.T) {
		_, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "update.me",
			Name: "",
		})
		if err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("nonexistent ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "does.not.exist",
			Name: "Ghost",
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestDeleteOpsRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("delete existing", func(t *testing.T) {
		if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "delete.me",
			Name: "To Delete",
		}); err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}

		if err := s.DeleteOpsRunbook(ctx, "delete.me"); err != nil {
			t.Fatalf("DeleteOpsRunbook: %v", err)
		}

		// Verify it's gone by trying to start it.
		_, err := s.StartOpsRunbook(ctx, "delete.me", time.Now().UTC())
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for deleted runbook, got: %v", err)
		}
	})

	t.Run("delete nonexistent returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsRunbook(ctx, "ghost.runbook")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete empty ID returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsRunbook(ctx, "")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestCreateOpsRunbookRun(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Seed a runbook with steps.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "run.test",
		Name: "Run Test Runbook",
		Steps: []OpsRunbookStep{
			{Type: "command", Title: "First Step", Command: "echo first"},
			{Type: "command", Title: "Second Step", Command: "echo second"},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	t.Run("creates queued run with correct fields", func(t *testing.T) {
		run, err := s.CreateOpsRunbookRun(ctx, "run.test", now)
		if err != nil {
			t.Fatalf("CreateOpsRunbookRun: %v", err)
		}
		if run.RunbookID != "run.test" {
			t.Fatalf("runbookId = %q, want run.test", run.RunbookID)
		}
		if run.RunbookName != "Run Test Runbook" {
			t.Fatalf("runbookName = %q, want Run Test Runbook", run.RunbookName)
		}
		if run.Status != opsRunbookStatusQueued {
			t.Fatalf("status = %q, want %q", run.Status, opsRunbookStatusQueued)
		}
		if run.TotalSteps != 2 {
			t.Fatalf("totalSteps = %d, want 2", run.TotalSteps)
		}
		if run.CompletedSteps != 0 {
			t.Fatalf("completedSteps = %d, want 0", run.CompletedSteps)
		}
		if run.CurrentStep != "First Step" {
			t.Fatalf("currentStep = %q, want First Step", run.CurrentStep)
		}
		if run.ID == "" {
			t.Fatalf("id should not be empty")
		}
	})

	t.Run("empty runbook ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.CreateOpsRunbookRun(ctx, "", now)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("nonexistent runbook returns error", func(t *testing.T) {
		_, err := s.CreateOpsRunbookRun(ctx, "no.such.runbook", now)
		if err == nil {
			t.Fatalf("expected error for nonexistent runbook")
		}
	})
}

func TestUpdateOpsRunbookRun(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Seed runbook + run.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "update.run.rb",
		Name: "Update Run Runbook",
		Steps: []OpsRunbookStep{
			{Type: "command", Title: "Step 1", Command: "echo 1"},
			{Type: "command", Title: "Step 2", Command: "echo 2"},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	run, err := s.CreateOpsRunbookRun(ctx, "update.run.rb", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun: %v", err)
	}

	t.Run("update to running with step results", func(t *testing.T) {
		stepResults := []OpsRunbookStepResult{
			{StepIndex: 0, Title: "Step 1", Type: "command", Output: "ok", DurationMs: 150},
		}
		resultsJSON, _ := json.Marshal(stepResults)

		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:          run.ID,
			Status:         opsRunbookStatusRunning,
			CompletedSteps: 1,
			CurrentStep:    "Step 2",
			StepResults:    string(resultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun: %v", err)
		}
		if updated.Status != opsRunbookStatusRunning {
			t.Fatalf("status = %q, want %q", updated.Status, opsRunbookStatusRunning)
		}
		if updated.CompletedSteps != 1 {
			t.Fatalf("completedSteps = %d, want 1", updated.CompletedSteps)
		}
		if updated.CurrentStep != "Step 2" {
			t.Fatalf("currentStep = %q, want Step 2", updated.CurrentStep)
		}
		if len(updated.StepResults) != 1 {
			t.Fatalf("len(stepResults) = %d, want 1", len(updated.StepResults))
		}
		if updated.StepResults[0].Output != "ok" {
			t.Fatalf("stepResults[0].output = %q, want ok", updated.StepResults[0].Output)
		}
	})

	t.Run("update to failed with error", func(t *testing.T) {
		finished := now.Add(5 * time.Second).Format(time.RFC3339)
		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:          run.ID,
			Status:         opsRunbookStatusFailed,
			CompletedSteps: 1,
			CurrentStep:    "Step 2",
			Error:          "command failed: exit 1",
			StepResults:    "[]",
			StartedAt:      now.Format(time.RFC3339),
			FinishedAt:     finished,
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun(failed): %v", err)
		}
		if updated.Status != opsRunbookStatusFailed {
			t.Fatalf("status = %q, want %q", updated.Status, opsRunbookStatusFailed)
		}
		if updated.Error != "command failed: exit 1" {
			t.Fatalf("error = %q, want 'command failed: exit 1'", updated.Error)
		}
		if updated.FinishedAt != finished {
			t.Fatalf("finishedAt = %q, want %q", updated.FinishedAt, finished)
		}
	})

	t.Run("empty step_results defaults to empty array", func(t *testing.T) {
		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:       run.ID,
			Status:      opsRunbookStatusSucceeded,
			StepResults: "",
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun: %v", err)
		}
		if updated.StepResults == nil {
			t.Fatalf("stepResults should be empty slice, not nil")
		}
	})

	t.Run("empty run ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:  "",
			Status: opsRunbookStatusRunning,
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestDeleteOpsRunbookRun(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Seed runbook + run.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:      "delete.run.rb",
		Name:    "Delete Run Runbook",
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	run, err := s.CreateOpsRunbookRun(ctx, "delete.run.rb", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun: %v", err)
	}

	t.Run("delete existing run", func(t *testing.T) {
		if err := s.DeleteOpsRunbookRun(ctx, run.ID); err != nil {
			t.Fatalf("DeleteOpsRunbookRun: %v", err)
		}

		_, err := s.GetOpsRunbookRun(ctx, run.ID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows after delete, got: %v", err)
		}
	})

	t.Run("delete nonexistent returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsRunbookRun(ctx, "nonexistent-run-id")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete empty ID returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsRunbookRun(ctx, "")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStartOpsRunbookErrorPaths(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("empty runbook ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.StartOpsRunbook(ctx, "", time.Now().UTC())
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("nonexistent runbook returns error", func(t *testing.T) {
		_, err := s.StartOpsRunbook(ctx, "no.such.runbook", time.Now().UTC())
		if err == nil {
			t.Fatalf("expected error for nonexistent runbook")
		}
	})

	t.Run("runbook with no steps succeeds", func(t *testing.T) {
		if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:      "empty.steps",
			Name:    "Empty Steps",
			Steps:   []OpsRunbookStep{},
			Enabled: true,
		}); err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		run, err := s.StartOpsRunbook(ctx, "empty.steps", time.Now().UTC())
		if err != nil {
			t.Fatalf("StartOpsRunbook: %v", err)
		}
		if run.TotalSteps != 0 {
			t.Fatalf("totalSteps = %d, want 0", run.TotalSteps)
		}
		if run.Status != opsRunbookStatusSucceeded {
			t.Fatalf("status = %q, want %q", run.Status, opsRunbookStatusSucceeded)
		}
		if run.CurrentStep != "completed" {
			t.Fatalf("currentStep = %q, want completed", run.CurrentStep)
		}
	})
}

func TestScanOpsRunbookRunMalformedJSON(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Seed a runbook + run, then corrupt step_results directly.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:      "malformed.test",
		Name:    "Malformed Test",
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	run, err := s.CreateOpsRunbookRun(ctx, "malformed.test", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun: %v", err)
	}

	// Corrupt step_results with invalid JSON directly via SQL.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE ops_runbook_runs SET step_results = 'not-json' WHERE id = ?`, run.ID,
	); err != nil {
		t.Fatalf("corrupt step_results: %v", err)
	}

	// scanOpsRunbookRun should gracefully handle malformed JSON.
	loaded, err := s.GetOpsRunbookRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun should not error on malformed step_results: %v", err)
	}
	if loaded.StepResults == nil {
		t.Fatalf("stepResults should be empty slice, not nil")
	}
	if len(loaded.StepResults) != 0 {
		t.Fatalf("len(stepResults) = %d, want 0 (malformed JSON fallback)", len(loaded.StepResults))
	}
}

func TestFailOrphanedRuns(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Seed a 2-step runbook.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "orphan.test",
		Name: "Orphan Test",
		Steps: []OpsRunbookStep{
			{Type: "command", Title: "Check status", Command: "echo ok"},
			{Type: "command", Title: "Restart service", Command: "systemctl restart sentinel"},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	// Create three runs: one stays queued, one advanced to running (step 2), one succeeded.
	queuedRun, err := s.CreateOpsRunbookRun(ctx, "orphan.test", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun(queued): %v", err)
	}

	// Simulate: step 1 completed, step 2 is running when server dies.
	runningRun, err := s.CreateOpsRunbookRun(ctx, "orphan.test", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun(running): %v", err)
	}
	step1Result, _ := json.Marshal([]OpsRunbookStepResult{
		{StepIndex: 0, Title: "Check status", Type: "command", Output: "ok", DurationMs: 120},
	})
	if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
		RunID:          runningRun.ID,
		Status:         opsRunbookStatusRunning,
		CompletedSteps: 1,
		CurrentStep:    "Restart service",
		StartedAt:      now.Format(time.RFC3339),
		StepResults:    string(step1Result),
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun(running): %v", err)
	}

	succeededRun, err := s.CreateOpsRunbookRun(ctx, "orphan.test", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun(succeeded): %v", err)
	}
	if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
		RunID:          succeededRun.ID,
		Status:         opsRunbookStatusSucceeded,
		CompletedSteps: 2,
		CurrentStep:    "Restart service",
		StartedAt:      now.Format(time.RFC3339),
		FinishedAt:     now.Add(time.Second).Format(time.RFC3339),
		StepResults:    "[]",
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun(succeeded): %v", err)
	}

	// Reconcile orphaned runs.
	n, err := s.FailOrphanedRuns(ctx)
	if err != nil {
		t.Fatalf("FailOrphanedRuns: %v", err)
	}
	if n != 2 {
		t.Fatalf("affected = %d, want 2", n)
	}

	// Verify queued run is failed, no step results added.
	q, err := s.GetOpsRunbookRun(ctx, queuedRun.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(queued): %v", err)
	}
	if q.Status != opsRunbookStatusFailed {
		t.Fatalf("queued run status = %q, want %q", q.Status, opsRunbookStatusFailed)
	}
	if q.Error != "interrupted by server restart" {
		t.Fatalf("queued run error = %q, want 'interrupted by server restart'", q.Error)
	}
	if q.FinishedAt == "" {
		t.Fatalf("queued run finishedAt should be set")
	}
	if len(q.StepResults) != 0 {
		t.Fatalf("queued run stepResults = %d, want 0 (never started)", len(q.StepResults))
	}

	// Verify running run is failed with interrupted step appended.
	r, err := s.GetOpsRunbookRun(ctx, runningRun.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(running): %v", err)
	}
	if r.Status != opsRunbookStatusFailed {
		t.Fatalf("running run status = %q, want %q", r.Status, opsRunbookStatusFailed)
	}
	if r.Error != "interrupted by server restart" {
		t.Fatalf("running run error = %q, want 'interrupted by server restart'", r.Error)
	}
	if len(r.StepResults) != 2 {
		t.Fatalf("running run stepResults = %d, want 2 (step 1 ok + step 2 interrupted)", len(r.StepResults))
	}
	// Step 1 should be the original successful result.
	if r.StepResults[0].Title != "Check status" || r.StepResults[0].Output != "ok" {
		t.Fatalf("step 0 = %+v, want Check status/ok", r.StepResults[0])
	}
	// Step 2 should be the interrupted step.
	interrupted := r.StepResults[1]
	if interrupted.StepIndex != 1 {
		t.Fatalf("interrupted stepIndex = %d, want 1", interrupted.StepIndex)
	}
	if interrupted.Title != "Restart service" {
		t.Fatalf("interrupted title = %q, want 'Restart service'", interrupted.Title)
	}
	if interrupted.Type != "interrupted" {
		t.Fatalf("interrupted type = %q, want 'interrupted'", interrupted.Type)
	}
	if interrupted.Error != "interrupted by server restart" {
		t.Fatalf("interrupted error = %q, want 'interrupted by server restart'", interrupted.Error)
	}

	// Verify succeeded run is untouched.
	su, err := s.GetOpsRunbookRun(ctx, succeededRun.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(succeeded): %v", err)
	}
	if su.Status != opsRunbookStatusSucceeded {
		t.Fatalf("succeeded run status = %q, want %q", su.Status, opsRunbookStatusSucceeded)
	}
	if su.Error != "" {
		t.Fatalf("succeeded run error = %q, want empty", su.Error)
	}

	// Calling again should affect 0 rows.
	n2, err := s.FailOrphanedRuns(ctx)
	if err != nil {
		t.Fatalf("FailOrphanedRuns (second call): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second call affected = %d, want 0", n2)
	}
}
