package api

import (
	"context"
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
	activityRows := int64(-1)
	for _, raw := range resources {
		resource := raw.(map[string]any)
		if resource["resource"] == store.StorageResourceActivityLog {
			activityRows = int64(resource["rows"].(float64))
		}
	}
	if activityRows != 1 {
		t.Fatalf("activity journal rows = %d, want 1; resources=%+v", activityRows, resources)
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
	if result["resource"] != store.StorageResourceActivityLog || result["removedRows"] != float64(1) {
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
