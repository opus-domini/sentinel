package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// ---------------------------------------------------------------------------
// reconcileManagedTmuxWindows with live rows
// ---------------------------------------------------------------------------

func TestReconcileManagedTmuxWindowsWithRows(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
	ctx := context.Background()

	row, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     "dev",
		LauncherName:    "Codex",
		Icon:            "code",
		WindowName:      "codex",
		TmuxWindowID:    "@10",
		LastWindowIndex: 1,
	})
	if err != nil {
		t.Fatalf("CreateManagedTmuxWindow: %v", err)
	}

	// Live window reports a different index than stored -> triggers runtime update.
	live := []tmux.Window{{ID: "@10", Index: 3, Name: "codex"}}
	filtered, err := h.reconcileManagedTmuxWindows(ctx, "dev", live)
	if err != nil {
		t.Fatalf("reconcileManagedTmuxWindows: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered rows = %d, want 1", len(filtered))
	}
	if filtered[0].LastWindowIndex != 3 {
		t.Fatalf("LastWindowIndex = %d, want 3", filtered[0].LastWindowIndex)
	}

	// When the runtime window disappears, the managed row is pruned.
	pruned, err := h.reconcileManagedTmuxWindows(ctx, "dev", nil)
	if err != nil {
		t.Fatalf("reconcileManagedTmuxWindows(empty): %v", err)
	}
	if len(pruned) != 0 {
		t.Fatalf("pruned rows = %d, want 0", len(pruned))
	}
	if _, getErr := st.ListManagedTmuxWindowsBySession(ctx, "dev"); getErr != nil {
		t.Fatalf("ListManagedTmuxWindowsBySession: %v", getErr)
	}
	_ = row
}

func TestManagedTmuxWindowForIndexReturnsManagedRow(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{ID: "@10", Index: 0, Name: "codex"}}, nil
		},
	}
	h, st := newTestHandler(t, tm)
	ctx := context.Background()
	if _, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     "dev",
		LauncherName:    "Codex",
		Icon:            "code",
		WindowName:      "codex",
		TmuxWindowID:    "@10",
		LastWindowIndex: 0,
	}); err != nil {
		t.Fatalf("CreateManagedTmuxWindow: %v", err)
	}

	row, ok, err := h.managedTmuxWindowForIndex(ctx, "dev", 0)
	if err != nil {
		t.Fatalf("managedTmuxWindowForIndex: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want managed window found")
	}
	if row.WindowName != "codex" {
		t.Fatalf("WindowName = %q, want codex", row.WindowName)
	}
}

// ---------------------------------------------------------------------------
// markSessionSeen full flow
// ---------------------------------------------------------------------------

func TestMarkSessionSeenScopes(t *testing.T) {
	t.Parallel()

	t.Run("session scope succeeds", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		h.events = events.NewHub()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"session"}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("window scope succeeds", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		h.events = events.NewHub()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"window","windowIndex":0}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("pane scope succeeds", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		h.events = events.NewHub()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"pane","paneId":"%1"}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects invalid session name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/bad/mark-seen", strings.NewReader(`{"scope":"session"}`))
		r.SetPathValue("session", "bad name")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`bad`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects missing scope", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects pane scope without percent prefix", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"pane","paneId":"7"}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects window scope with negative index", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"window","windowIndex":-1}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects unknown scope", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/mark-seen", strings.NewReader(`{"scope":"galaxy"}`))
		r.SetPathValue("session", "dev")
		h.markSessionSeen(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// runbook create / run flows
// ---------------------------------------------------------------------------

func TestCreateOpsRunbookSuccess(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/runbooks", strings.NewReader(
		`{"name":"deploy","steps":[{"type":"run","title":"echo","command":"echo hello"}]}`))
	h.createOpsRunbook(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}

	runbooks, err := st.ListOpsRunbooks(context.Background())
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	found := false
	for _, rb := range runbooks {
		if rb.Name == "deploy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("runbooks = %#v, want one named deploy", runbooks)
	}
}

func TestCreateOpsRunbookRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/runbooks", strings.NewReader(`bad`))
	h.createOpsRunbook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRunOpsRunbookRejectsMissingRequiredParameter(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.events = events.NewHub()
	ctx := context.Background()
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "param-runbook",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "x", Command: "echo $name"}},
		Parameters: []store.RunbookParameter{
			{Name: "name", Label: "Name", Type: "string", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/runbooks/"+rb.ID+"/run", strings.NewReader(`{"parameters":{}}`))
	r.SetPathValue("runbook", rb.ID)
	h.runOpsRunbook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestRunOpsRunbookRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/runbooks/abc/run", strings.NewReader(`{bad`))
	r.SetPathValue("runbook", "abc")
	h.runOpsRunbook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRunOpsRunbookSuccessWithParameters(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.events = events.NewHub()
	ctx := context.Background()
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "param-runbook",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "x", Command: "true"}},
		Parameters: []store.RunbookParameter{
			{Name: "name", Label: "Name", Type: "string", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/runbooks/"+rb.ID+"/run", strings.NewReader(`{"parameters":{"name":"world"}}`))
	r.SetPathValue("runbook", rb.ID)
	h.runOpsRunbook(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	h.wg.Wait()
}

// ---------------------------------------------------------------------------
// guardrail / marker list flows with seeded data
// ---------------------------------------------------------------------------

func TestListGuardrailRulesWithData(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.guardrails = guardrails.New(st)
	if err := st.UpsertGuardrailRule(context.Background(), store.GuardrailRuleWrite{
		ID:      "rule-1",
		Name:    "Block rm -rf",
		Pattern: "rm -rf",
		Mode:    store.GuardrailModeBlock,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/guardrails", nil)
	h.listGuardrailRules(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	rules, _ := data["rules"].([]any)
	found := false
	for _, item := range rules {
		rule, _ := item.(map[string]any)
		if rule["id"] == "rule-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rules = %#v, want seeded rule-1", rules)
	}
}

func TestListGuardrailRulesNilGuardrails(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.guardrails = nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/guardrails", nil)
	h.listGuardrailRules(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestListMarkerPatternsNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.repo = nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/markers", nil)
	h.listMarkerPatterns(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestCreateGuardrailRuleSuccess(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.guardrails = guardrails.New(st)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/guardrails", strings.NewReader(
		`{"id":"r1","name":"block","pattern":"rm -rf","mode":"block","enabled":true}`))
	h.createGuardrailRule(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}

	rules, err := st.ListGuardrailRules(context.Background())
	if err != nil {
		t.Fatalf("ListGuardrailRules: %v", err)
	}
	found := false
	for _, rule := range rules {
		if rule.ID == "r1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rules = %#v, want rule r1", rules)
	}
}

func TestDeleteGuardrailRuleSuccess(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.guardrails = guardrails.New(st)
	if err := st.UpsertGuardrailRule(context.Background(), store.GuardrailRuleWrite{
		ID:      "del-me",
		Pattern: "danger",
		Mode:    store.GuardrailModeWarn,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/ops/guardrails/del-me", nil)
	r.SetPathValue("rule", "del-me")
	h.deleteGuardrailRule(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// activity stats / timeline search success paths
// ---------------------------------------------------------------------------

func TestActivityStatsNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.repo = nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/activity/stats", nil)
	h.activityStats(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestTimelineSearchSuccess(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search?limit=10&since="+
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), nil)
	h.timelineSearch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// listSessions projection path
// ---------------------------------------------------------------------------

func TestListSessionsFromProjection(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		Windows:     1,
		Panes:       1,
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	h.listSessions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	sessions, _ := data["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
}

func TestListSessionsFromProjectionOverlaysLiveTmuxSessions(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{
				{Name: "dev", Windows: 1, CreatedAt: now, ActivityAt: now},
				{Name: "fresh", Windows: 1, CreatedAt: now, ActivityAt: now},
			}, nil
		},
		listPanesFn: func(_ context.Context, session string) ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: session, PaneID: "%1"}}, nil
		},
	}
	h, st := newTestHandler(t, tm)
	ctx := context.Background()
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		Windows:     1,
		Panes:       1,
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	h.listSessions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	sessions, _ := data["sessions"].([]any)
	found := map[string]bool{}
	for _, raw := range sessions {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		found[name] = true
	}
	if !found["dev"] || !found["fresh"] {
		t.Fatalf("sessions = %v, want projected and live-only sessions", found)
	}
}

func TestListSessionsFromTmuxFallback(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{Name: "dev", Windows: 1, CreatedAt: time.Now(), ActivityAt: time.Now()}}, nil
		},
	}
	h, _ := newTestHandler(t, tm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	h.listSessions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	sessions, _ := data["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
}

func TestListSessionsFromTmuxError(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h, _ := newTestHandler(t, tm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	h.listSessions(w, r)
	if w.Code == http.StatusOK {
		t.Fatalf("status = %d, want error", w.Code)
	}
}

// ---------------------------------------------------------------------------
// reorder error paths
// ---------------------------------------------------------------------------

func TestReorderHandlersRejectBadInput(t *testing.T) {
	t.Parallel()

	t.Run("reorder sessions rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/order", strings.NewReader(`bad`))
		h.reorderSessions(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder sessions rejects empty names", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/order", strings.NewReader(`{"names":[]}`))
		h.reorderSessions(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder sessions rejects duplicate names", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/order", strings.NewReader(`{"names":["a","a"]}`))
		h.reorderSessions(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder session presets rejects bad name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/order", strings.NewReader(`{"names":["bad name"]}`))
		h.reorderSessionPresets(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder windows rejects invalid session", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/bad/windows/order", strings.NewReader(`{"windowIds":["@1"]}`))
		r.SetPathValue("session", "bad name")
		h.reorderWindows(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder windows rejects empty window ids", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/dev/windows/order", strings.NewReader(`{"windowIds":[]}`))
		r.SetPathValue("session", "dev")
		h.reorderWindows(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder windows rejects stale order", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
				return []tmux.Window{{ID: "@1", Index: 0}, {ID: "@2", Index: 1}}, nil
			},
		}
		h, _ := newTestHandler(t, tm)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/dev/windows/order", strings.NewReader(`{"windowIds":["@1"]}`))
		r.SetPathValue("session", "dev")
		h.reorderWindows(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("reorder windows surfaces tmux list failure", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, _ := newTestHandler(t, tm)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/dev/windows/order", strings.NewReader(`{"windowIds":["@1"]}`))
		r.SetPathValue("session", "dev")
		h.reorderWindows(w, r)
		if w.Code == http.StatusNoContent {
			t.Fatalf("status = %d, want error", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// pure helper coverage
// ---------------------------------------------------------------------------

func TestSortSessionsByStoredOrder(t *testing.T) {
	t.Parallel()

	sessions := []enrichedSession{
		{Name: "zeta", SortOrder: 0},
		{Name: "alpha", SortOrder: 0},
		{Name: "two", SortOrder: 2},
		{Name: "one", SortOrder: 1},
	}
	sortSessionsByStoredOrder(sessions)
	want := []string{"one", "two", "alpha", "zeta"}
	for i, name := range want {
		if sessions[i].Name != name {
			t.Fatalf("sessions[%d] = %q, want %q (got %#v)", i, sessions[i].Name, name, sessions)
		}
	}
}

func TestSameProjectedPaneSet(t *testing.T) {
	t.Parallel()

	t.Run("empty returns false", func(t *testing.T) {
		t.Parallel()
		if sameProjectedPaneSet(nil, nil) {
			t.Fatal("empty pane sets should not match")
		}
	})

	t.Run("length mismatch returns false", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Pane{{PaneID: "%1"}}
		if sameProjectedPaneSet(live, nil) {
			t.Fatal("length mismatch should not match")
		}
	})

	t.Run("id mismatch returns false", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Pane{{PaneID: "%1"}, {PaneID: "%2"}}
		projected := []store.WatchtowerPane{{PaneID: "%1"}, {PaneID: "%9"}}
		if sameProjectedPaneSet(live, projected) {
			t.Fatal("differing pane ids should not match")
		}
	})

	t.Run("matching set returns true", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Pane{{PaneID: "%1"}, {PaneID: "%2"}}
		projected := []store.WatchtowerPane{{PaneID: "%2"}, {PaneID: "%1"}}
		if !sameProjectedPaneSet(live, projected) {
			t.Fatal("matching pane ids should match")
		}
	})
}

func TestDecodeSessionPresetWriteDefaults(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"api"}`))
	row, err := decodeSessionPresetWrite(r)
	if err != nil {
		t.Fatalf("decodeSessionPresetWrite: %v", err)
	}
	if row.Cwd == "" {
		t.Fatal("Cwd should default to a non-empty path")
	}
	if row.Icon != defaultSessionPresetIcon {
		t.Fatalf("Icon = %q, want default %q", row.Icon, defaultSessionPresetIcon)
	}
}

func TestCreateTmuxLauncherRejectsDuplicate(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
	if _, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
		Name:       "Codex",
		Icon:       "code",
		Command:    "codex",
		CwdMode:    store.TmuxLauncherCwdModeSession,
		WindowName: "codex",
	}); err != nil {
		t.Fatalf("CreateTmuxLauncher: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(
		`{"name":"Codex","icon":"code","command":"codex","cwdMode":"session","windowName":"codex"}`))
	h.createTmuxLauncher(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestIsTmuxLauncherValidationError(t *testing.T) {
	t.Parallel()

	if isTmuxLauncherValidationError(nil) {
		t.Fatal("nil error should not be a validation error")
	}
	if isTmuxLauncherValidationError(errors.New("some other error")) {
		t.Fatal("unrelated error should not be a validation error")
	}
	if !isTmuxLauncherValidationError(errors.New("tmux launcher name is required")) {
		t.Fatal("known validation message should be recognized")
	}
}
