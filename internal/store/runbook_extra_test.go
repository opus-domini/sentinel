package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestGetOpsRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Seeded runbooks should exist.
	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatal("expected at least one seeded runbook")
	}

	// Fetch by ID.
	got, err := s.GetOpsRunbook(ctx, runbooks[0].ID)
	if err != nil {
		t.Fatalf("GetOpsRunbook: %v", err)
	}
	if got.ID != runbooks[0].ID {
		t.Fatalf("id = %q, want %q", got.ID, runbooks[0].ID)
	}
	if got.Name == "" {
		t.Fatal("name is empty")
	}
}

func TestGetOpsRunbookEmptyID(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.GetOpsRunbook(context.Background(), "")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestGetOpsRunbookNonexistent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.GetOpsRunbook(context.Background(), "nonexistent-runbook-id")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestRunbookParametersRoundTrip(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	t.Run("insert and get with parameters", func(t *testing.T) {
		params := []RunbookParameter{
			{Name: "host", Label: "Target Host", Type: "string", Required: true},
			{Name: "port", Label: "Port", Type: "number", Default: "8080"},
			{Name: "env", Label: "Environment", Type: "select", Required: true, Options: []string{"dev", "staging", "prod"}},
			{Name: "verbose", Label: "Verbose", Type: "boolean", Default: "false"},
		}
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:         "params.test",
			Name:       "Parameterized Runbook",
			Parameters: params,
			Steps: []OpsRunbookStep{
				{Type: "run", Title: "Deploy", Command: "deploy.sh {{host}} {{port}}"},
			},
			Enabled: true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if len(rb.Parameters) != 4 {
			t.Fatalf("len(parameters) = %d, want 4", len(rb.Parameters))
		}
		if rb.Parameters[0].Name != "host" {
			t.Fatalf("params[0].name = %q, want host", rb.Parameters[0].Name)
		}
		if !rb.Parameters[0].Required {
			t.Fatalf("params[0].required = false, want true")
		}
		if rb.Parameters[1].Default != "8080" {
			t.Fatalf("params[1].default = %q, want 8080", rb.Parameters[1].Default)
		}
		if len(rb.Parameters[2].Options) != 3 {
			t.Fatalf("params[2].options = %v, want 3 elements", rb.Parameters[2].Options)
		}

		// Verify GetOpsRunbook also returns parameters.
		got, err := s.GetOpsRunbook(ctx, "params.test")
		if err != nil {
			t.Fatalf("GetOpsRunbook: %v", err)
		}
		if len(got.Parameters) != 4 {
			t.Fatalf("get: len(parameters) = %d, want 4", len(got.Parameters))
		}
	})

	t.Run("update parameters", func(t *testing.T) {
		updated, err := s.UpdateOpsRunbook(ctx, OpsRunbookWrite{
			ID:   "params.test",
			Name: "Parameterized Runbook",
			Parameters: []RunbookParameter{
				{Name: "host", Label: "Host", Type: "string", Required: true},
			},
		})
		if err != nil {
			t.Fatalf("UpdateOpsRunbook: %v", err)
		}
		if len(updated.Parameters) != 1 {
			t.Fatalf("len(parameters) = %d, want 1", len(updated.Parameters))
		}
	})

	t.Run("nil parameters stored as empty array", func(t *testing.T) {
		rb, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
			ID:         "params.nil",
			Name:       "No Params",
			Parameters: nil,
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		if rb.Parameters == nil {
			t.Fatalf("parameters should be empty slice, not nil")
		}
		if len(rb.Parameters) != 0 {
			t.Fatalf("len(parameters) = %d, want 0", len(rb.Parameters))
		}
	})

	t.Run("list includes parameters", func(t *testing.T) {
		all, err := s.ListOpsRunbooks(ctx)
		if err != nil {
			t.Fatalf("ListOpsRunbooks: %v", err)
		}
		var found bool
		for _, r := range all {
			if r.ID == "params.test" {
				found = true
				if len(r.Parameters) != 1 {
					t.Fatalf("list: len(parameters) = %d, want 1 (after update)", len(r.Parameters))
				}
			}
		}
		if !found {
			t.Fatal("params.test not found in list")
		}
	})
}

func TestRunbookParametersBackwardCompatibility(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Seeded runbooks (inserted without explicit parameters column) should
	// have empty parameters arrays.
	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	for _, rb := range runbooks {
		if rb.Parameters == nil {
			t.Fatalf("runbook %q: parameters should be empty slice, not nil", rb.ID)
		}
		if len(rb.Parameters) != 0 {
			t.Fatalf("runbook %q: len(parameters) = %d, want 0", rb.ID, len(rb.Parameters))
		}
	}

	// Runs without parameters should also have empty map.
	run, err := s.StartOpsRunbook(ctx, runbooks[0].ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("StartOpsRunbook: %v", err)
	}
	if run.ParametersUsed == nil {
		t.Fatal("parametersUsed should be empty map, not nil")
	}
	if len(run.ParametersUsed) != 0 {
		t.Fatalf("len(parametersUsed) = %d, want 0", len(run.ParametersUsed))
	}
}

func TestCreateOpsRunbookRunWithParams(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	// Seed runbook with parameters.
	if _, err := s.InsertOpsRunbook(ctx, OpsRunbookWrite{
		ID:   "run.params.test",
		Name: "Params Run Test",
		Parameters: []RunbookParameter{
			{Name: "host", Label: "Host", Type: "string", Required: true},
			{Name: "env", Label: "Environment", Type: "select", Required: true, Options: []string{"dev", "prod"}},
		},
		Steps: []OpsRunbookStep{
			{Type: "run", Title: "Deploy", Command: "deploy.sh {{host}} --env={{env}}"},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	t.Run("stores parameters in run", func(t *testing.T) {
		params := map[string]string{"host": "server.example.com", "env": "prod"}
		run, err := s.CreateOpsRunbookRunWithParams(ctx, "run.params.test", now, params)
		if err != nil {
			t.Fatalf("CreateOpsRunbookRunWithParams: %v", err)
		}
		if run.ParametersUsed == nil {
			t.Fatal("parametersUsed should not be nil")
		}
		if run.ParametersUsed["host"] != "server.example.com" {
			t.Fatalf("parametersUsed[host] = %q, want server.example.com", run.ParametersUsed["host"])
		}
		if run.ParametersUsed["env"] != "prod" {
			t.Fatalf("parametersUsed[env] = %q, want prod", run.ParametersUsed["env"])
		}
		if run.Status != opsRunbookStatusQueued {
			t.Fatalf("status = %q, want %q", run.Status, opsRunbookStatusQueued)
		}

		// Verify persisted via GetOpsRunbookRun.
		loaded, err := s.GetOpsRunbookRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetOpsRunbookRun: %v", err)
		}
		if loaded.ParametersUsed["host"] != "server.example.com" {
			t.Fatalf("loaded parametersUsed[host] = %q", loaded.ParametersUsed["host"])
		}
	})

	t.Run("nil params stored as empty map", func(t *testing.T) {
		run, err := s.CreateOpsRunbookRunWithParams(ctx, "run.params.test", now, nil)
		if err != nil {
			t.Fatalf("CreateOpsRunbookRunWithParams: %v", err)
		}
		if run.ParametersUsed == nil {
			t.Fatal("parametersUsed should be empty map, not nil")
		}
		if len(run.ParametersUsed) != 0 {
			t.Fatalf("len(parametersUsed) = %d, want 0", len(run.ParametersUsed))
		}
	})

	t.Run("empty runbook ID returns ErrNoRows", func(t *testing.T) {
		_, err := s.CreateOpsRunbookRunWithParams(ctx, "", now, nil)
		if err != sql.ErrNoRows {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("nonexistent runbook returns error", func(t *testing.T) {
		_, err := s.CreateOpsRunbookRunWithParams(ctx, "no.such.runbook", now, nil)
		if err == nil {
			t.Fatal("expected error for nonexistent runbook")
		}
	})

	t.Run("parameters visible in run list", func(t *testing.T) {
		runs, err := s.ListOpsRunbookRuns(ctx, 100)
		if err != nil {
			t.Fatalf("ListOpsRunbookRuns: %v", err)
		}
		var found bool
		for _, r := range runs {
			if r.ParametersUsed != nil && r.ParametersUsed["host"] == "server.example.com" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("run with host parameter not found in list")
		}
	})
}

func TestSuggestRunbooksForMarker(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Seed runbooks with varied names and descriptions.
	seeds := []OpsRunbookWrite{
		{ID: "rb-error-fix", Name: "Fix OOM Error", Description: "Handles out-of-memory errors on production", Enabled: true},
		{ID: "rb-deploy", Name: "Deploy Service", Description: "Deploys a service to the target host", Enabled: true},
		{ID: "rb-restart-dev", Name: "Restart dev", Description: "Restarts the dev session services", Enabled: true},
		{ID: "rb-timeout-handler", Name: "Handle Timeout", Description: "Investigates timeout errors in the pipeline", Enabled: true},
		{ID: "rb-disabled", Name: "Disabled Error Handler", Description: "Should not appear because it is disabled", Enabled: false},
		{ID: "rb-cleanup", Name: "Cleanup Logs", Description: "Removes stale log files from disk", Enabled: true},
		{ID: "rb-error-alert", Name: "Error Alert Triage", Description: "Triage process for error alerts", Enabled: true},
	}
	for _, seed := range seeds {
		if _, err := s.InsertOpsRunbook(ctx, seed); err != nil {
			t.Fatalf("InsertOpsRunbook(%s): %v", seed.ID, err)
		}
	}

	tests := []struct {
		name       string
		marker     string
		session    string
		wantIDs    []string // expected IDs in result (order may vary for same relevance)
		wantAbsent []string // IDs that must NOT appear
	}{
		{
			name:       "match marker in name",
			marker:     "error",
			session:    "",
			wantIDs:    []string{"rb-error-fix", "rb-error-alert"},
			wantAbsent: []string{"rb-disabled"},
		},
		{
			name:    "match marker in description",
			marker:  "timeout",
			session: "",
			wantIDs: []string{"rb-timeout-handler"},
		},
		{
			name:    "match session name",
			marker:  "",
			session: "dev",
			wantIDs: []string{"rb-restart-dev"},
		},
		{
			name:    "match both marker and session",
			marker:  "error",
			session: "dev",
			wantIDs: []string{"rb-error-fix", "rb-error-alert", "rb-restart-dev"},
		},
		{
			name:    "no matches",
			marker:  "nonexistent-keyword-xyz",
			session: "nonexistent-session-abc",
			wantIDs: []string{},
		},
		{
			name:    "empty inputs return empty",
			marker:  "",
			session: "",
			wantIDs: []string{},
		},
		{
			name:       "disabled runbooks excluded",
			marker:     "disabled",
			session:    "",
			wantAbsent: []string{"rb-disabled"},
		},
		{
			name:    "max 5 results",
			marker:  "e", // broad match, will hit many runbooks
			session: "",
			// Just verify we get at most 5
		},
		{
			name:    "case insensitive matching",
			marker:  "OOM",
			session: "",
			wantIDs: []string{"rb-error-fix"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results, err := s.SuggestRunbooksForMarker(ctx, tc.marker, tc.session)
			if err != nil {
				t.Fatalf("SuggestRunbooksForMarker(%q, %q): %v", tc.marker, tc.session, err)
			}

			if len(results) > 5 {
				t.Fatalf("returned %d results, want at most 5", len(results))
			}

			resultIDs := make(map[string]bool, len(results))
			for _, r := range results {
				resultIDs[r.ID] = true
			}

			for _, wantID := range tc.wantIDs {
				if !resultIDs[wantID] {
					t.Errorf("expected runbook %q in results, got IDs: %v", wantID, mapKeys(resultIDs))
				}
			}

			for _, absentID := range tc.wantAbsent {
				if resultIDs[absentID] {
					t.Errorf("runbook %q should NOT appear in results", absentID)
				}
			}
		})
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
