package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestStorageStatsAndFlushActivityJournal(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	if _, err := st.InsertWatchtowerJournal(ctx, store.WatchtowerJournalWrite{
		GlobalRev:  1,
		EntityType: "session",
		Session:    "dev",
		WindowIdx:  -1,
		ChangeKind: "activity",
		ChangedAt:  time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal: %v", err)
	}

	w := httptest.NewRecorder()
	h.storageStats(w, httptest.NewRequest(http.MethodGet, "/api/ops/storage/stats", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("storageStats status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	stats := jsonBody(t, w)["data"].(map[string]any)
	resources := stats["resources"].([]any)
	var activity map[string]any
	for _, raw := range resources {
		resource := raw.(map[string]any)
		if resource["resource"] == store.StorageResourceActivityLog {
			activity = resource
		}
	}
	if activity == nil ||
		activity["totalRows"] != float64(1) ||
		activity["flushableRows"] != float64(1) ||
		activity["protectedRows"] != float64(0) {
		t.Fatalf("activity journal stats = %+v, want total=1 flushable=1 protected=0", activity)
	}

	w = httptest.NewRecorder()
	h.flushStorage(w, httptest.NewRequest(http.MethodPost, "/api/ops/storage/flush", strings.NewReader(`{"resource":"activity-journal"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("flushStorage status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	data := jsonBody(t, w)["data"].(map[string]any)
	if data["flushedAt"] == "" {
		t.Fatalf("flushedAt is empty: %+v", data)
	}
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("flush results len = %d, want 1", len(results))
	}
	result := results[0].(map[string]any)
	if result["resource"] != store.StorageResourceActivityLog ||
		result["removedRows"] != float64(1) ||
		result["protectedRows"] != float64(0) {
		t.Fatalf("flush result = %+v, want activity journal removedRows=1", result)
	}

	remaining, err := st.ListWatchtowerJournalSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining journal rows = %d, want 0", len(remaining))
	}
}

func TestStorageStatsAndFlushJobsPreserveActiveRuns(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	runbook, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		ID:   "storage.api.jobs",
		Name: "Storage API Jobs",
		Steps: []store.OpsRunbookStep{{
			Type: "run", Title: "Run", Command: "true",
		}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	statuses := []string{
		store.OpsRunbookStatusQueued,
		store.OpsRunbookStatusRunning,
		store.OpsRunbookStatusWaitingApproval,
		store.OpsRunbookStatusSucceeded,
		store.OpsRunbookStatusFailed,
	}
	runIDs := make(map[string]string, len(statuses))
	base := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	for idx, status := range statuses {
		run, createErr := st.CreateOpsRunbookRun(ctx, store.OpsRunbookRunWrite{
			Definition: runbook,
			Source:     store.OpsRunbookRunSourceRunbooks,
			At:         base.Add(time.Duration(idx) * time.Minute),
		})
		if createErr != nil {
			t.Fatalf("CreateOpsRunbookRun(%s): %v", status, createErr)
		}
		if status != store.OpsRunbookStatusQueued {
			run, createErr = st.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
				RunID:  run.ID,
				Status: status,
			})
			if createErr != nil {
				t.Fatalf("UpdateOpsRunbookRun(%s): %v", status, createErr)
			}
		}
		runIDs[status] = run.ID
	}

	w := httptest.NewRecorder()
	h.storageStats(w, httptest.NewRequest(http.MethodGet, "/api/ops/storage/stats", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("storageStats status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	jobs := findStorageResource(t, jsonBody(t, w), store.StorageResourceOpsJobs)
	if jobs["totalRows"] != float64(5) ||
		jobs["flushableRows"] != float64(2) ||
		jobs["protectedRows"] != float64(3) {
		t.Fatalf("jobs stats = %+v, want total=5 flushable=2 protected=3", jobs)
	}

	w = httptest.NewRecorder()
	h.flushStorage(w, httptest.NewRequest(
		http.MethodPost,
		"/api/ops/storage/flush",
		strings.NewReader(`{"resource":"ops-jobs"}`),
	))
	if w.Code != http.StatusOK {
		t.Fatalf("flushStorage status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	results := jsonBody(t, w)["data"].(map[string]any)["results"].([]any)
	result := results[0].(map[string]any)
	if result["removedRows"] != float64(2) || result["protectedRows"] != float64(3) {
		t.Fatalf("flush result = %+v, want removed=2 protected=3", result)
	}

	for _, status := range statuses[:3] {
		if _, err := st.GetOpsRunbookRun(ctx, runIDs[status]); err != nil {
			t.Fatalf("protected status %q was removed: %v", status, err)
		}
	}
	for _, status := range statuses[3:] {
		if _, err := st.GetOpsRunbookRun(ctx, runIDs[status]); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("terminal status %q lookup error = %v, want sql.ErrNoRows", status, err)
		}
	}
}

func findStorageResource(
	t *testing.T,
	body map[string]any,
	resource string,
) map[string]any {
	t.Helper()
	data := body["data"].(map[string]any)
	for _, raw := range data["resources"].([]any) {
		item := raw.(map[string]any)
		if item["resource"] == resource {
			return item
		}
	}
	t.Fatalf("storage resource %q not found in %+v", resource, data["resources"])
	return nil
}
