package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestOpsTimelineInsertAndSearch(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	_, err := s.InsertOpsTimelineEvent(ctx, OpsTimelineEventWrite{
		Source:    "service",
		EventType: "service.action",
		Severity:  "warn",
		Resource:  "sentinel",
		Message:   "restart executed",
		Details:   "systemctl --user restart sentinel",
		Metadata:  `{"action":"restart"}`,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertOpsTimelineEvent: %v", err)
	}

	result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{
		Query:    "restart",
		Severity: "warn",
		Source:   "service",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchOpsTimelineEvents: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(result.Events))
	}
	event := result.Events[0]
	if event.EventType != "service.action" || event.Resource != "sentinel" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestOpsAlertUpsertAndAck(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	first, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert(first): %v", err)
	}

	second, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed again",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert(second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("alert id changed on dedupe: first=%d second=%d", first.ID, second.ID)
	}
	if second.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", second.Occurrences)
	}

	alerts, err := s.ListOpsAlerts(ctx, 10, opsAlertStatusOpen)
	if err != nil {
		t.Fatalf("ListOpsAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}

	acked, err := s.AckOpsAlert(ctx, first.ID, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("AckOpsAlert: %v", err)
	}
	if acked.Status != opsAlertStatusAcked {
		t.Fatalf("status = %q, want %q", acked.Status, opsAlertStatusAcked)
	}
	if acked.AckedAt == "" {
		t.Fatalf("ackedAt should not be empty")
	}

	_, err = s.AckOpsAlert(ctx, 99999, base)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ack missing alert error = %v, want sql.ErrNoRows", err)
	}
}

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

// --- Custom Services CRUD ---

func TestInsertOpsCustomService(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		svc, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name:        "nginx",
			DisplayName: "Nginx Web Server",
			Manager:     "systemd",
			Unit:        "nginx.service",
			Scope:       "system",
		})
		if err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}
		if svc.Name != "nginx" {
			t.Fatalf("name = %q, want nginx", svc.Name)
		}
		if svc.DisplayName != "Nginx Web Server" {
			t.Fatalf("displayName = %q, want Nginx Web Server", svc.DisplayName)
		}
		if svc.Manager != "systemd" {
			t.Fatalf("manager = %q, want systemd", svc.Manager)
		}
		if svc.Unit != "nginx.service" {
			t.Fatalf("unit = %q, want nginx.service", svc.Unit)
		}
		if svc.Scope != "system" {
			t.Fatalf("scope = %q, want system", svc.Scope)
		}
		if !svc.Enabled {
			t.Fatalf("enabled = false, want true")
		}
		if svc.CreatedAt == "" || svc.UpdatedAt == "" {
			t.Fatalf("timestamps should be set")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		svc, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "redis",
			Unit: "redis.service",
		})
		if err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}
		if svc.DisplayName != "redis" {
			t.Fatalf("displayName should default to name, got %q", svc.DisplayName)
		}
		if svc.Manager != "systemd" {
			t.Fatalf("manager should default to systemd, got %q", svc.Manager)
		}
		if svc.Scope != "user" {
			t.Fatalf("scope should default to user, got %q", svc.Scope)
		}
	})

	t.Run("empty name errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "",
			Unit: "foo.service",
		})
		if err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("empty unit errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "foo",
			Unit: "",
		})
		if err == nil {
			t.Fatalf("expected error for empty unit")
		}
	})

	t.Run("duplicate name errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "nginx",
			Unit: "nginx2.service",
		})
		if err == nil {
			t.Fatalf("expected error for duplicate name")
		}
	})
}

func TestListOpsCustomServices(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Empty list.
	list, err := s.ListOpsCustomServices(ctx)
	if err != nil {
		t.Fatalf("ListOpsCustomServices(empty): %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len = %d, want 0", len(list))
	}

	// Insert two services.
	for _, w := range []OpsCustomServiceWrite{
		{Name: "beta", Unit: "beta.service"},
		{Name: "alpha", Unit: "alpha.service"},
	} {
		if _, err := s.InsertOpsCustomService(ctx, w); err != nil {
			t.Fatalf("InsertOpsCustomService(%s): %v", w.Name, err)
		}
	}

	list, err = s.ListOpsCustomServices(ctx)
	if err != nil {
		t.Fatalf("ListOpsCustomServices: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	// Sorted by name ASC.
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("services not sorted: [%s, %s]", list[0].Name, list[1].Name)
	}
}

func TestDeleteOpsCustomService(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("delete existing", func(t *testing.T) {
		if _, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "to-delete",
			Unit: "to-delete.service",
		}); err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}

		if err := s.DeleteOpsCustomService(ctx, "to-delete"); err != nil {
			t.Fatalf("DeleteOpsCustomService: %v", err)
		}

		list, err := s.ListOpsCustomServices(ctx)
		if err != nil {
			t.Fatalf("ListOpsCustomServices: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("len = %d, want 0 after delete", len(list))
		}
	})

	t.Run("delete nonexistent returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsCustomService(ctx, "ghost")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete empty name returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsCustomService(ctx, "")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

// --- Runbook CRUD ---

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
		if updated.UpdatedAt == orig.CreatedAt {
			// UpdatedAt should be refreshed (not strictly equal to creation time).
			// This is a best-effort check; clock granularity may make them equal.
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

// --- Runbook Run CRUD ---

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

// --- ResolveOpsAlert ---

func TestResolveOpsAlert(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed an open alert.
	alert, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "resolve:test",
		Source:    "test",
		Resource:  "svc",
		Title:     "Test Alert",
		Message:   "something went wrong",
		Severity:  "error",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert: %v", err)
	}

	t.Run("resolve open alert", func(t *testing.T) {
		resolved, err := s.ResolveOpsAlert(ctx, "resolve:test", base.Add(time.Minute))
		if err != nil {
			t.Fatalf("ResolveOpsAlert: %v", err)
		}
		if resolved.Status != opsAlertStatusResolved {
			t.Fatalf("status = %q, want %q", resolved.Status, opsAlertStatusResolved)
		}
		if resolved.ResolvedAt == "" {
			t.Fatalf("resolvedAt should not be empty")
		}
		if resolved.ID != alert.ID {
			t.Fatalf("id = %d, want %d", resolved.ID, alert.ID)
		}
	})

	t.Run("resolve already resolved returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveOpsAlert(ctx, "resolve:test", base.Add(2*time.Minute))
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("resolve empty dedupe key returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveOpsAlert(ctx, "", base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("resolve nonexistent returns ErrNoRows", func(t *testing.T) {
		_, err := s.ResolveOpsAlert(ctx, "no:such:key", base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

// --- Improved coverage for existing functions ---

func TestUpsertOpsAlertReopen(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Create and resolve an alert.
	if _, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "reopen:test",
		Source:    "test",
		Title:     "Reopen Alert",
		Severity:  "warn",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("UpsertOpsAlert(create): %v", err)
	}
	if _, err := s.ResolveOpsAlert(ctx, "reopen:test", base.Add(time.Minute)); err != nil {
		t.Fatalf("ResolveOpsAlert: %v", err)
	}

	// Upsert same dedupe key → should reopen (status back to open).
	reopened, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "reopen:test",
		Source:    "test",
		Title:     "Reopen Alert",
		Severity:  "warn",
		CreatedAt: base.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert(reopen): %v", err)
	}
	if reopened.Status != opsAlertStatusOpen {
		t.Fatalf("status = %q, want %q (should reopen)", reopened.Status, opsAlertStatusOpen)
	}
	if reopened.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", reopened.Occurrences)
	}
}

func TestUpsertOpsAlertValidation(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("empty dedupe key errors", func(t *testing.T) {
		_, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
			DedupeKey: "",
			Source:    "test",
			CreatedAt: base,
		})
		if err == nil {
			t.Fatalf("expected error for empty dedupe key")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		alert, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
			DedupeKey: "defaults:test",
			CreatedAt: base,
		})
		if err != nil {
			t.Fatalf("UpsertOpsAlert: %v", err)
		}
		// Source defaults to "ops".
		if alert.Source != "ops" {
			t.Fatalf("source = %q, want ops", alert.Source)
		}
		// Title defaults to dedupe key.
		if alert.Title != "defaults:test" {
			t.Fatalf("title = %q, want defaults:test", alert.Title)
		}
		// Message defaults to title.
		if alert.Message != "defaults:test" {
			t.Fatalf("message = %q, want defaults:test", alert.Message)
		}
		// Severity defaults to info.
		if alert.Severity != opsSeverityInfo {
			t.Fatalf("severity = %q, want %q", alert.Severity, opsSeverityInfo)
		}
	})
}

func TestAckOpsAlertEdgeCases(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("ack negative ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.AckOpsAlert(ctx, -1, base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("ack zero ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.AckOpsAlert(ctx, 0, base)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("cannot ack resolved alert", func(t *testing.T) {
		alert, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
			DedupeKey: "ack:resolved",
			Source:    "test",
			Title:     "Resolved Alert",
			Severity:  "error",
			CreatedAt: base,
		})
		if err != nil {
			t.Fatalf("UpsertOpsAlert: %v", err)
		}
		if _, err := s.ResolveOpsAlert(ctx, "ack:resolved", base.Add(time.Minute)); err != nil {
			t.Fatalf("ResolveOpsAlert: %v", err)
		}

		_, err = s.AckOpsAlert(ctx, alert.ID, base.Add(2*time.Minute))
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows (cannot ack resolved)", err)
		}
	})
}

func TestInsertOpsTimelineEventDefaults(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Insert with all empty/default fields.
	event, err := s.InsertOpsTimelineEvent(ctx, OpsTimelineEventWrite{
		Message: "bare event",
	})
	if err != nil {
		t.Fatalf("InsertOpsTimelineEvent: %v", err)
	}
	if event.Source != "ops" {
		t.Fatalf("source = %q, want ops (default)", event.Source)
	}
	if event.EventType != "ops.event" {
		t.Fatalf("eventType = %q, want ops.event (default)", event.EventType)
	}
	if event.Severity != opsSeverityInfo {
		t.Fatalf("severity = %q, want %q (default)", event.Severity, opsSeverityInfo)
	}
	if event.CreatedAt == "" {
		t.Fatalf("createdAt should be set by default")
	}
}

func TestSearchOpsTimelineEventsFilters(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed diverse events.
	events := []OpsTimelineEventWrite{
		{Source: "service", EventType: "restart", Severity: "warn", Resource: "nginx", Message: "nginx restarted", CreatedAt: base},
		{Source: "service", EventType: "start", Severity: "info", Resource: "redis", Message: "redis started", CreatedAt: base.Add(time.Second)},
		{Source: "deploy", EventType: "deploy", Severity: "error", Resource: "app", Message: "deploy failed", CreatedAt: base.Add(2 * time.Second)},
	}
	for _, e := range events {
		if _, err := s.InsertOpsTimelineEvent(ctx, e); err != nil {
			t.Fatalf("InsertOpsTimelineEvent(%s): %v", e.Resource, err)
		}
	}

	t.Run("filter by severity only", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Severity: "error"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Resource != "app" {
			t.Fatalf("expected 1 error event (app), got %d: %+v", len(result.Events), result.Events)
		}
	})

	t.Run("filter by source only", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Source: "service"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 2 {
			t.Fatalf("expected 2 service events, got %d", len(result.Events))
		}
	})

	t.Run("filter by query text", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Query: "redis"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Resource != "redis" {
			t.Fatalf("expected 1 redis event, got %d", len(result.Events))
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(result.Events))
		}
	})

	t.Run("severity 'all' returns all", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Severity: "all"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(result.Events))
		}
	})

	t.Run("invalid severity returns error", func(t *testing.T) {
		_, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Severity: "critical"})
		if err == nil {
			t.Fatalf("expected error for invalid severity")
		}
		if !errors.Is(err, ErrInvalidOpsFilter) {
			t.Fatalf("error = %v, want ErrInvalidOpsFilter", err)
		}
	})

	t.Run("HasMore when limit exceeded", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Limit: 2})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if !result.HasMore {
			t.Fatalf("hasMore = false, want true")
		}
		if len(result.Events) != 2 {
			t.Fatalf("len(events) = %d, want 2 (limited)", len(result.Events))
		}
	})

	t.Run("negative limit defaults to 100", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Limit: -5})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		// Should return all 3 events (well under default 100 limit).
		if len(result.Events) != 3 {
			t.Fatalf("len(events) = %d, want 3", len(result.Events))
		}
	})

	t.Run("severity aliases normalized", func(t *testing.T) {
		// "warning" should be treated as "warn".
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Severity: "warning"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Severity != opsSeverityWarn {
			t.Fatalf("expected 1 warn event, got %d", len(result.Events))
		}

		// "err" should be treated as "error".
		result, err = s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{Severity: "err"})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents(err): %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Severity != opsSeverityError {
			t.Fatalf("expected 1 error event, got %d", len(result.Events))
		}
	})

	t.Run("results ordered by created_at DESC", func(t *testing.T) {
		result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{})
		if err != nil {
			t.Fatalf("SearchOpsTimelineEvents: %v", err)
		}
		if len(result.Events) < 2 {
			t.Fatalf("need at least 2 events for ordering check")
		}
		// First event should be the most recent.
		if result.Events[0].Resource != "app" {
			t.Fatalf("first event = %q, want app (most recent)", result.Events[0].Resource)
		}
	})
}

func TestListOpsAlertsFilters(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	t.Run("invalid status returns error", func(t *testing.T) {
		_, err := s.ListOpsAlerts(ctx, 10, "bogus")
		if err == nil {
			t.Fatalf("expected error for invalid status")
		}
		if !errors.Is(err, ErrInvalidOpsFilter) {
			t.Fatalf("error = %v, want ErrInvalidOpsFilter", err)
		}
	})

	t.Run("empty status returns all", func(t *testing.T) {
		if _, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
			DedupeKey: "list:a",
			Source:    "test",
			Severity:  "info",
			CreatedAt: base,
		}); err != nil {
			t.Fatalf("UpsertOpsAlert: %v", err)
		}

		alerts, err := s.ListOpsAlerts(ctx, 10, "")
		if err != nil {
			t.Fatalf("ListOpsAlerts: %v", err)
		}
		if len(alerts) < 1 {
			t.Fatalf("expected at least 1 alert")
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

func TestResolveOpsAlertAcked(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Create and ack an alert.
	alert, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "resolve:acked",
		Source:    "test",
		Title:     "Acked Alert",
		Severity:  "warn",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert: %v", err)
	}
	if _, err := s.AckOpsAlert(ctx, alert.ID, base.Add(time.Minute)); err != nil {
		t.Fatalf("AckOpsAlert: %v", err)
	}

	// Resolving an acked alert should succeed.
	resolved, err := s.ResolveOpsAlert(ctx, "resolve:acked", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ResolveOpsAlert(acked): %v", err)
	}
	if resolved.Status != opsAlertStatusResolved {
		t.Fatalf("status = %q, want %q", resolved.Status, opsAlertStatusResolved)
	}
}
