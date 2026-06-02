package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestParseActivityDeltaParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantSince int64
		wantLimit int
		wantErr   bool
	}{
		{name: "defaults", target: "/api/activity/delta", wantSince: 0, wantLimit: 200},
		{name: "valid", target: "/api/activity/delta?since=7&limit=25", wantSince: 7, wantLimit: 25},
		{name: "clamps limit", target: "/api/activity/delta?since=3&limit=5001", wantSince: 3, wantLimit: 1000},
		{name: "invalid since text", target: "/api/activity/delta?since=nope", wantErr: true},
		{name: "invalid since negative", target: "/api/activity/delta?since=-1", wantErr: true},
		{name: "invalid limit text", target: "/api/activity/delta?limit=nope", wantErr: true},
		{name: "invalid limit zero", target: "/api/activity/delta?limit=0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			gotSince, gotLimit, err := parseActivityDeltaParams(req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseActivityDeltaParams error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseActivityDeltaParams error = %v", err)
			}
			if gotSince != tt.wantSince || gotLimit != tt.wantLimit {
				t.Fatalf("parseActivityDeltaParams = (%d, %d), want (%d, %d)", gotSince, gotLimit, tt.wantSince, tt.wantLimit)
			}
		})
	}
}

func TestExtractChangedSessionNames(t *testing.T) {
	t.Parallel()

	got := extractChangedSessionNames([]store.WatchtowerJournal{
		{Session: " dev "},
		{Session: ""},
		{Session: "\t"},
		{Session: "prod"},
		{Session: "dev"},
	})
	sort.Strings(got)
	want := []string{"dev", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractChangedSessionNames = %v, want %v", got, want)
	}
}

func TestActivityDeltaSuccessIncludesPatchesAndOverflow(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	seedActivityDeltaSession(t, st, "dev", "%1", now, 11)
	seedActivityDeltaSession(t, st, "prod", "%2", now.Add(time.Minute), 12)
	if err := st.SetWatchtowerRuntimeValue(ctx, "global_rev", "12"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue: %v", err)
	}
	for _, row := range []store.WatchtowerJournalWrite{
		{GlobalRev: 10, EntityType: "session", Session: "dev", ChangeKind: "updated", ChangedAt: now},
		{GlobalRev: 11, EntityType: "pane", Session: "dev", WindowIdx: 0, PaneID: "%1", ChangeKind: "updated", ChangedAt: now.Add(time.Second)},
		{GlobalRev: 12, EntityType: "session", Session: "prod", ChangeKind: "updated", ChangedAt: now.Add(2 * time.Second)},
	} {
		if _, err := st.InsertWatchtowerJournal(ctx, row); err != nil {
			t.Fatalf("InsertWatchtowerJournal(%d): %v", row.GlobalRev, err)
		}
	}

	w := httptest.NewRecorder()
	h.activityDelta(w, httptest.NewRequest(http.MethodGet, "/api/activity/delta?since=9&limit=2", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("activityDelta status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	data := jsonBody(t, w)["data"].(map[string]any)
	if data["since"] != float64(9) || data["limit"] != float64(2) || data[keyGlobalRev] != float64(12) || data["overflow"] != true {
		t.Fatalf("unexpected delta metadata: %+v", data)
	}
	changes := data["changes"].([]any)
	if len(changes) != 2 {
		t.Fatalf("len(changes) = %d, want 2", len(changes))
	}
	assertPatchSessions(t, data["sessionPatches"], []string{"dev"})
	assertInspectorPatchSessions(t, data["inspectorPatches"], []string{"dev"})

	w = httptest.NewRecorder()
	h.activityDelta(w, httptest.NewRequest(http.MethodGet, "/api/activity/delta?since=9&limit=3", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("activityDelta status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	data = jsonBody(t, w)["data"].(map[string]any)
	if data["overflow"] != false {
		t.Fatalf("overflow = %v, want false", data["overflow"])
	}
	assertPatchSessions(t, data["sessionPatches"], []string{"dev", "prod"})
	assertInspectorPatchSessions(t, data["inspectorPatches"], []string{"dev", "prod"})
}

func TestActivityDeltaBadRequest(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	h.activityDelta(w, httptest.NewRequest(http.MethodGet, "/api/activity/delta?limit=0", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("activityDelta status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if got := errCode(jsonBody(t, w)); got != invalidRequestCode {
		t.Fatalf("error code = %q, want %q", got, invalidRequestCode)
	}
}

func TestActivityDeltaNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.repo = nil
	w := httptest.NewRecorder()
	h.activityDelta(w, httptest.NewRequest(http.MethodGet, "/api/activity/delta", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("activityDelta status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestActivityStatsSuccessParsesRuntime(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	values := map[string]string{
		"global_rev":                    " 42 ",
		"collect_total":                 "7",
		"collect_errors_total":          "invalid",
		"last_collect_at":               "2026-06-02T12:00:00Z",
		"last_collect_duration_ms":      "bad-duration",
		"last_collect_sessions":         "3",
		"last_collect_changed_sessions": "2",
		"last_collect_error":            "last error",
	}
	if err := st.SetWatchtowerRuntimeValues(ctx, values); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValues: %v", err)
	}

	w := httptest.NewRecorder()
	h.activityStats(w, httptest.NewRequest(http.MethodGet, "/api/activity/stats", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("activityStats status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	data := jsonBody(t, w)["data"].(map[string]any)
	if data[keyGlobalRev] != float64(42) || data["collectTotal"] != float64(7) || data["collectErrorsTotal"] != float64(0) || data["lastCollectDurationMs"] != float64(0) || data["lastCollectSessions"] != float64(3) || data["lastCollectChanged"] != float64(2) {
		t.Fatalf("unexpected parsed stats: %+v", data)
	}
	if data["lastCollectAt"] != "2026-06-02T12:00:00Z" || data["lastCollectError"] != "last error" {
		t.Fatalf("unexpected string stats: %+v", data)
	}
	runtime := data["runtime"].(map[string]any)
	if runtime["global_rev"] != "42" || runtime["collect_errors_total"] != "invalid" || runtime["last_collect_error"] != "last error" {
		t.Fatalf("unexpected runtime map: %+v", runtime)
	}
}

func seedActivityDeltaSession(t *testing.T, st *store.Store, session, paneID string, now time.Time, rev int64) {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{SessionName: session, Attached: 1, Windows: 1, Panes: 1, ActivityAt: now, LastPreview: "preview " + session, LastPreviewAt: now, LastPreviewPaneID: paneID, UnreadWindows: 1, UnreadPanes: 1, Rev: rev}); err != nil {
		t.Fatalf("UpsertWatchtowerSession(%s): %v", session, err)
	}
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{SessionName: session, TmuxWindowID: "@" + session, WindowIndex: 0, Name: "main", Active: true, Layout: "layout", WindowActivityAt: now, UnreadPanes: 1, HasUnread: true, Rev: rev}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow(%s): %v", session, err)
	}
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{PaneID: paneID, SessionName: session, WindowIndex: 0, PaneIndex: 0, Title: "shell", Active: true, TailHash: "hash", TailPreview: "line", TailCapturedAt: now, Revision: rev, SeenRevision: rev - 1, ChangedAt: now}); err != nil {
		t.Fatalf("UpsertWatchtowerPane(%s): %v", session, err)
	}
}

func assertPatchSessions(t *testing.T, raw any, want []string) {
	t.Helper()
	patches := raw.([]any)
	got := make([]string, 0, len(patches))
	for _, patch := range patches {
		got = append(got, patch.(map[string]any)["name"].(string))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("session patch names = %v, want %v", got, want)
	}
}

func assertInspectorPatchSessions(t *testing.T, raw any, want []string) {
	t.Helper()
	patches := raw.([]any)
	got := make([]string, 0, len(patches))
	for _, patch := range patches {
		got = append(got, patch.(map[string]any)["session"].(string))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inspector patch sessions = %v, want %v", got, want)
	}
}
