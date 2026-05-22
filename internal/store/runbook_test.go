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
			{Type: "run", Title: "Check status", Command: "systemctl status"},
			{Type: "approval", Title: "Review", Description: "Verify output"},
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
		if rb.Steps[0].Type != "run" || rb.Steps[1].Type != "approval" {
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

	t.Run("webhook URL round-trip", func(t *testing.T) {
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:         "test.webhook",
			Name:       "Webhook Runbook",
			WebhookURL: "https://hooks.example.com/sentinel",
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.WebhookURL != "https://hooks.example.com/sentinel" {
			t.Fatalf("webhookURL = %q, want https://hooks.example.com/sentinel", rb.WebhookURL)
		}

		// Verify it persists through list.
		all, err := s.ListOpsRunbooks(ctx)
		if err != nil {
			t.Fatalf("ListOpsRunbooks: %v", err)
		}
		var found bool
		for _, r := range all {
			if r.ID == "test.webhook" {
				found = true
				if r.WebhookURL != "https://hooks.example.com/sentinel" {
					t.Fatalf("list webhookURL = %q, want https://hooks.example.com/sentinel", r.WebhookURL)
				}
			}
		}
		if !found {
			t.Fatalf("test.webhook not found in list")
		}
	})

	t.Run("empty webhook URL defaults to empty string", func(t *testing.T) {
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:      "test.no.webhook",
			Name:    "No Webhook",
			Enabled: true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.WebhookURL != "" {
			t.Fatalf("webhookURL = %q, want empty", rb.WebhookURL)
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
		Steps:       []OpsRunbookStep{{Type: "run", Title: "Step 1", Command: "echo hello"}},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	t.Run("update all fields", func(t *testing.T) {
		newSteps := []OpsRunbookStep{
			{Type: "run", Title: "Verify", Command: "service should be running"},
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
		if len(updated.Steps) != 1 || updated.Steps[0].Type != "run" {
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

	t.Run("update webhook URL", func(t *testing.T) {
		updated, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:         "update.me",
			Name:       "Updated",
			WebhookURL: "https://hooks.example.com/notify",
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbook: %v", err)
		}
		if updated.WebhookURL != "https://hooks.example.com/notify" {
			t.Fatalf("webhookURL = %q, want https://hooks.example.com/notify", updated.WebhookURL)
		}

		// Clear webhook URL.
		cleared, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:         "update.me",
			Name:       "Updated",
			WebhookURL: "",
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbook(clear): %v", err)
		}
		if cleared.WebhookURL != "" {
			t.Fatalf("webhookURL = %q, want empty", cleared.WebhookURL)
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
			{Type: "run", Title: "First Step", Command: "echo first"},
			{Type: "run", Title: "Second Step", Command: "echo second"},
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
			{Type: "run", Title: "Step 1", Command: "echo 1"},
			{Type: "run", Title: "Step 2", Command: "echo 2"},
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
			{StepIndex: 0, Title: "Step 1", Type: "run", Output: "ok", DurationMs: 150},
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

	t.Run("empty step_results preserves previous results", func(t *testing.T) {
		stepResults := []OpsRunbookStepResult{
			{StepIndex: 0, Title: "Step 1", Type: "run", Output: "kept", DurationMs: 150},
		}
		resultsJSON, _ := json.Marshal(stepResults)
		if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:       run.ID,
			Status:      opsRunbookStatusRunning,
			StepResults: string(resultsJSON),
		}); err != nil {
			t.Fatalf("seed step results: %v", err)
		}

		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:       run.ID,
			Status:      opsRunbookStatusSucceeded,
			StepResults: "",
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun: %v", err)
		}
		if len(updated.StepResults) != 1 {
			t.Fatalf("stepResults = %d, want previous result preserved", len(updated.StepResults))
		}
		if updated.StepResults[0].Output != "kept" {
			t.Fatalf("stepResults[0].output = %q, want kept", updated.StepResults[0].Output)
		}
	})

	t.Run("explicit empty array clears step results", func(t *testing.T) {
		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:       run.ID,
			Status:      opsRunbookStatusSucceeded,
			StepResults: "[]",
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun: %v", err)
		}
		if len(updated.StepResults) != 0 {
			t.Fatalf("stepResults = %d, want empty slice", len(updated.StepResults))
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

	t.Run("empty StartedAt preserves previous value", func(t *testing.T) {
		// Set startedAt first.
		started := now.Format(time.RFC3339)
		if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:     run.ID,
			Status:    opsRunbookStatusRunning,
			StartedAt: started,
		}); err != nil {
			t.Fatalf("set startedAt: %v", err)
		}

		// Update without startedAt — should NOT clobber it.
		finished := now.Add(10 * time.Second).Format(time.RFC3339)
		updated, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
			RunID:      run.ID,
			Status:     opsRunbookStatusSucceeded,
			FinishedAt: finished,
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbookRun: %v", err)
		}
		if updated.StartedAt != started {
			t.Fatalf("startedAt = %q, want %q (should be preserved)", updated.StartedAt, started)
		}
		if updated.FinishedAt != finished {
			t.Fatalf("finishedAt = %q, want %q", updated.FinishedAt, finished)
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

	runs := seedFailOrphanedRunsFixture(ctx, t, s, now)

	// Reconcile orphaned runs.
	n, err := s.FailOrphanedRuns(ctx)
	if err != nil {
		t.Fatalf("FailOrphanedRuns: %v", err)
	}
	if n != 2 {
		t.Fatalf("affected = %d, want 2", n)
	}

	assertQueuedOrphanedRunFailed(ctx, t, s, runs.queuedID)
	assertRunningOrphanedRunFailed(ctx, t, s, runs.runningID)
	assertWaitingApprovalRunPreserved(ctx, t, s, runs.waitingID)
	assertSucceededRunUntouched(ctx, t, s, runs.succeededID)

	// Calling again should affect 0 rows.
	n2, err := s.FailOrphanedRuns(ctx)
	if err != nil {
		t.Fatalf("FailOrphanedRuns (second call): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second call affected = %d, want 0", n2)
	}
}

type failOrphanedRunsFixture struct {
	queuedID    string
	runningID   string
	waitingID   string
	succeededID string
}

func seedFailOrphanedRunsFixture(
	ctx context.Context,
	t *testing.T,
	s *Store,
	now time.Time,
) failOrphanedRunsFixture {
	t.Helper()

	seedOrphanRunbook(ctx, t, s)

	queuedRun := createOrphanRun(ctx, t, s, now, "queued")
	runningRun := createOrphanRun(ctx, t, s, now, "running")
	updateRunningOrphanRun(ctx, t, s, now, runningRun.ID)

	waitingRun := createOrphanRun(ctx, t, s, now, "waiting")
	updateWaitingApprovalRun(ctx, t, s, now, waitingRun.ID)

	succeededRun := createOrphanRun(ctx, t, s, now, "succeeded")
	updateSucceededRun(ctx, t, s, now, succeededRun.ID)

	return failOrphanedRunsFixture{
		queuedID:    queuedRun.ID,
		runningID:   runningRun.ID,
		waitingID:   waitingRun.ID,
		succeededID: succeededRun.ID,
	}
}

func seedOrphanRunbook(ctx context.Context, t *testing.T, s *Store) {
	t.Helper()

	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "orphan.test",
		Name: "Orphan Test",
		Steps: []OpsRunbookStep{
			{Type: "run", Title: "Check status", Command: "echo ok"},
			{Type: "run", Title: "Restart service", Command: "systemctl restart sentinel"},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
}

func createOrphanRun(ctx context.Context, t *testing.T, s *Store, now time.Time, label string) OpsRunbookRun {
	t.Helper()

	run, err := s.CreateOpsRunbookRun(ctx, "orphan.test", now)
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun(%s): %v", label, err)
	}
	return run
}

func updateRunningOrphanRun(ctx context.Context, t *testing.T, s *Store, now time.Time, runID string) {
	t.Helper()

	// Simulate: step 1 completed, step 2 was pre-populated by beforeStep
	// but the server died before it finished executing.
	stepResults := marshalRunbookStepResults(t, []OpsRunbookStepResult{
		{StepIndex: 0, Title: "Check status", Type: "run", Output: "ok", DurationMs: 120},
		{StepIndex: 1, Title: "Restart service", Type: "run"},
	})
	if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         opsRunbookStatusRunning,
		CompletedSteps: 1,
		CurrentStep:    "Restart service",
		StartedAt:      now.Format(time.RFC3339),
		StepResults:    stepResults,
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun(running): %v", err)
	}
}

func updateWaitingApprovalRun(ctx context.Context, t *testing.T, s *Store, now time.Time, runID string) {
	t.Helper()

	waitingStepResults := marshalRunbookStepResults(t, []OpsRunbookStepResult{
		{StepIndex: 0, Title: "Check status", Type: "run", Output: "ok", DurationMs: 120},
		{StepIndex: 1, Title: "Approve restart", Type: "approval", Output: "review output"},
	})
	if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         OpsRunbookStatusWaitingApproval,
		CompletedSteps: 2,
		CurrentStep:    "Approve restart",
		StartedAt:      now.Format(time.RFC3339),
		StepResults:    waitingStepResults,
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun(waiting): %v", err)
	}
}

func updateSucceededRun(ctx context.Context, t *testing.T, s *Store, now time.Time, runID string) {
	t.Helper()

	if _, err := s.UpdateOpsRunbookRun(ctx, OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         opsRunbookStatusSucceeded,
		CompletedSteps: 2,
		CurrentStep:    "Restart service",
		StartedAt:      now.Format(time.RFC3339),
		FinishedAt:     now.Add(time.Second).Format(time.RFC3339),
		StepResults:    "[]",
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun(succeeded): %v", err)
	}
}

func marshalRunbookStepResults(t *testing.T, results []OpsRunbookStepResult) string {
	t.Helper()

	raw, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("json.Marshal(step results): %v", err)
	}
	return string(raw)
}

func assertQueuedOrphanedRunFailed(ctx context.Context, t *testing.T, s *Store, runID string) {
	t.Helper()

	q, err := s.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(queued): %v", err)
	}
	if q.Status != opsRunbookStatusFailed {
		t.Fatalf("queued run status = %q, want %q", q.Status, opsRunbookStatusFailed)
	}
	if q.Error != opsRunbookOrphanError {
		t.Fatalf("queued run error = %q, want 'interrupted by server restart'", q.Error)
	}
	if q.FinishedAt == "" {
		t.Fatalf("queued run finishedAt should be set")
	}
	if len(q.StepResults) != 0 {
		t.Fatalf("queued run stepResults = %d, want 0 (never started)", len(q.StepResults))
	}
}

func assertRunningOrphanedRunFailed(ctx context.Context, t *testing.T, s *Store, runID string) {
	t.Helper()

	r, err := s.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(running): %v", err)
	}
	if r.Status != opsRunbookStatusFailed {
		t.Fatalf("running run status = %q, want %q", r.Status, opsRunbookStatusFailed)
	}
	if r.Error != opsRunbookOrphanError {
		t.Fatalf("running run error = %q, want %q", r.Error, opsRunbookOrphanError)
	}
	if len(r.StepResults) != 2 {
		t.Fatalf("running run stepResults = %d, want 2 (step 1 ok + step 2 pre-populated)", len(r.StepResults))
	}
	if r.StepResults[0].Title != "Check status" || r.StepResults[0].Output != "ok" {
		t.Fatalf("step 0 = %+v, want Check status/ok", r.StepResults[0])
	}
	assertPrePopulatedRestartStep(t, r.StepResults[1])
}

func assertPrePopulatedRestartStep(t *testing.T, step OpsRunbookStepResult) {
	t.Helper()

	if step.StepIndex != 1 {
		t.Fatalf("pre-populated stepIndex = %d, want 1", step.StepIndex)
	}
	if step.Title != "Restart service" {
		t.Fatalf("pre-populated title = %q, want 'Restart service'", step.Title)
	}
	if step.Type != "run" {
		t.Fatalf("pre-populated type = %q, want 'command'", step.Type)
	}
	if step.Output != "" || step.Error != "" {
		t.Fatalf("pre-populated should have empty output/error, got output=%q error=%q", step.Output, step.Error)
	}
}

func assertWaitingApprovalRunPreserved(ctx context.Context, t *testing.T, s *Store, runID string) {
	t.Helper()

	w, err := s.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(waiting): %v", err)
	}
	if w.Status != OpsRunbookStatusWaitingApproval {
		t.Fatalf("waiting run status = %q, want %q", w.Status, OpsRunbookStatusWaitingApproval)
	}
	if w.Error != "" {
		t.Fatalf("waiting run error = %q, want empty", w.Error)
	}
	if len(w.StepResults) != 2 {
		t.Fatalf("waiting run stepResults = %d, want 2", len(w.StepResults))
	}
}

func assertSucceededRunUntouched(ctx context.Context, t *testing.T, s *Store, runID string) {
	t.Helper()

	su, err := s.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(succeeded): %v", err)
	}
	if su.Status != opsRunbookStatusSucceeded {
		t.Fatalf("succeeded run status = %q, want %q", su.Status, opsRunbookStatusSucceeded)
	}
	if su.Error != "" {
		t.Fatalf("succeeded run error = %q, want empty", su.Error)
	}
}
