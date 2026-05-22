package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// ---------------------------------------------------------------------------
// runbook list / delete flows
// ---------------------------------------------------------------------------

func TestOpsRunbooksListSuccess(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	if _, err := st.InsertOpsRunbook(context.Background(), store.OpsRunbookWrite{
		Name:  "deploy",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "x", Command: "true"}},
	}); err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/runbooks", nil)
	h.opsRunbooks(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if _, ok := data["runbooks"]; !ok {
		t.Fatal("response missing runbooks key")
	}
	if _, ok := data["jobs"]; !ok {
		t.Fatal("response missing jobs key")
	}
	if _, ok := data["schedules"]; !ok {
		t.Fatal("response missing schedules key")
	}
}

func TestDeleteOpsRunbookSuccessWithScheduleCascade(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.events = events.NewHub()
	ctx := context.Background()
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "cascade",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "x", Command: "true"}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	if _, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "nightly",
		ScheduleType: scheduleTypeCron,
		CronExpr:     "0 0 * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/ops/runbooks/"+rb.ID, nil)
	r.SetPathValue("runbook", rb.ID)
	h.deleteOpsRunbook(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	if len(schedules) != 0 {
		t.Fatalf("schedules = %#v, want cascaded delete", schedules)
	}
}

// ---------------------------------------------------------------------------
// tmux session handler flows
// ---------------------------------------------------------------------------

func TestCreateSessionRejectsDisallowedUser(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	// Default guard rejects any non-empty target user.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(`{"name":"dev","cwd":"/tmp","user":"postgres"}`))
	h.createSession(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateSessionRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(`bad`))
	h.createSession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateSessionRejectsRelativeCwd(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(`{"name":"dev","cwd":"relative"}`))
	h.createSession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRenameSessionMigratesPreset(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{}
	h, st := newTestHandler(t, tm)
	h.events = events.NewHub()
	ctx := context.Background()
	if _, err := st.CreateSessionPreset(ctx, store.SessionPresetWrite{
		Name: "api", Cwd: "/srv/api", Icon: "server",
	}); err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/api/rename", strings.NewReader(`{"newName":"web"}`))
	r.SetPathValue("session", "api")
	h.renameSession(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	presets, err := st.ListSessionPresets(ctx)
	if err != nil {
		t.Fatalf("ListSessionPresets: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "web" {
		t.Fatalf("presets = %#v, want preset renamed to web", presets)
	}
}

func TestRenameSessionRejectsInvalidNewName(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/api/rename", strings.NewReader(`{"newName":"bad name"}`))
	r.SetPathValue("session", "api")
	h.renameSession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRenameSessionSurfacesTmuxError(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		renameSessionFn: func(context.Context, string, string) error {
			return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
		},
	}
	h, _ := newTestHandler(t, tm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/api/rename", strings.NewReader(`{"newName":"web"}`))
	r.SetPathValue("session", "api")
	h.renameSession(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestSetSessionIconRejectsBadIcon(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/tmux/sessions/api/icon", strings.NewReader(`{"icon":"BAD ICON"}`))
	r.SetPathValue("session", "api")
	h.setSessionIcon(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSetSessionIconRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/tmux/sessions/api/icon", strings.NewReader(`bad`))
	r.SetPathValue("session", "api")
	h.setSessionIcon(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestDeleteSessionRejectsInvalidName(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/bad", nil)
	r.SetPathValue("session", "bad name")
	h.deleteSession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestDeleteSessionSurfacesTmuxError(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		killSessionFn: func(context.Context, string) error {
			return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h, _ := newTestHandler(t, tm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/dev", nil)
	r.SetPathValue("session", "dev")
	h.deleteSession(w, r)
	if w.Code == http.StatusNoContent {
		t.Fatalf("status = %d, want error", w.Code)
	}
}

func TestDeleteSessionTolerantOfMissingSession(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		killSessionFn: func(context.Context, string) error {
			return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
		},
	}
	h, _ := newTestHandler(t, tm)
	h.events = events.NewHub()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/dev", nil)
	r.SetPathValue("session", "dev")
	h.deleteSession(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestFrequentDirectoriesCapsLimit(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/frequent-directories?limit=999", nil)
	h.frequentDirectories(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ---------------------------------------------------------------------------
// tmuxForUser / SessionUser registry
// ---------------------------------------------------------------------------

func TestTmuxForUserAndSessionUser(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})

	// Empty user returns the handler's default tmux service.
	if got := h.tmuxForUser(""); got != h.tmux {
		t.Fatal("tmuxForUser(\"\") should return default tmux service")
	}
	// Non-empty user returns a wrapped service.
	if _, ok := h.tmuxForUser("postgres").(tmux.Service); !ok {
		t.Fatal("tmuxForUser(user) should return tmux.Service")
	}

	if got := h.SessionUser("unknown"); got != "" {
		t.Fatalf("SessionUser(unknown) = %q, want empty", got)
	}

	h.registerSessionUser("dev", "postgres")
	if got := h.SessionUser("dev"); got != "postgres" {
		t.Fatalf("SessionUser(dev) = %q, want postgres", got)
	}
	// Empty user is a no-op.
	h.registerSessionUser("noop", "")
	if got := h.SessionUser("noop"); got != "" {
		t.Fatalf("SessionUser(noop) = %q, want empty", got)
	}

	users := h.knownSessionUsers()
	if len(users) != 1 || users[0] != "postgres" {
		t.Fatalf("knownSessionUsers = %#v, want [postgres]", users)
	}
}

// ---------------------------------------------------------------------------
// nil-repo guards across handlers
// ---------------------------------------------------------------------------

func TestHandlersRejectNilRepo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(h *Handler, w http.ResponseWriter, r *http.Request)
		req  func() *http.Request
	}{
		{
			name: "opsRunbooks",
			call: (*Handler).opsRunbooks,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/runbooks", nil) },
		},
		{
			name: "updateOpsRunbook",
			call: (*Handler).updateOpsRunbook,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/api/ops/runbooks/x", strings.NewReader(`{}`))
			},
		},
		{
			name: "deleteOpsRunbook",
			call: (*Handler).deleteOpsRunbook,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodDelete, "/api/ops/runbooks/x", nil) },
		},
		{
			name: "rejectOpsRunbookRun",
			call: (*Handler).rejectOpsRunbookRun,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodPost, "/api/ops/jobs/x/reject", nil) },
		},
		{
			name: "approveOpsRunbookRun",
			call: (*Handler).approveOpsRunbookRun,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodPost, "/api/ops/jobs/x/approve", nil) },
		},
		{
			name: "listSchedules",
			call: (*Handler).listSchedules,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/schedules", nil) },
		},
		{
			name: "createSchedule",
			call: (*Handler).createSchedule,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{}`))
			},
		},
		{
			name: "updateSchedule",
			call: (*Handler).updateSchedule,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/api/ops/schedules/x", strings.NewReader(`{}`))
			},
		},
		{
			name: "deleteSchedule",
			call: (*Handler).deleteSchedule,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodDelete, "/api/ops/schedules/x", nil) },
		},
		{
			name: "triggerSchedule",
			call: (*Handler).triggerSchedule,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodPost, "/api/ops/schedules/x/trigger", nil) },
		},
		{
			name: "deleteMarkerPattern",
			call: (*Handler).deleteMarkerPattern,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodDelete, "/api/ops/markers/x", nil) },
		},
		{
			name: "upsertMarkerPattern",
			call: (*Handler).upsertMarkerPattern,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/api/ops/markers/x", strings.NewReader(`{}`))
			},
		},
		{
			name: "activityStats",
			call: (*Handler).activityStats,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/activity/stats", nil) },
		},
		{
			name: "timelineSearch",
			call: (*Handler).timelineSearch,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search", nil) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler(t, nil)
			h.repo = nil
			w := httptest.NewRecorder()
			tc.call(h, w, tc.req())
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// guardrails service-unavailable guards
// ---------------------------------------------------------------------------

func TestGuardrailHandlersRejectNilGuardrails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(h *Handler, w http.ResponseWriter, r *http.Request)
		req  func() *http.Request
	}{
		{
			name: "updateGuardrailRule",
			call: (*Handler).updateGuardrailRule,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/api/ops/guardrails/x", strings.NewReader(`{}`))
			},
		},
		{
			name: "createGuardrailRule",
			call: (*Handler).createGuardrailRule,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/ops/guardrails", strings.NewReader(`{}`))
			},
		},
		{
			name: "deleteGuardrailRule",
			call: (*Handler).deleteGuardrailRule,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodDelete, "/api/ops/guardrails/x", nil) },
		},
		{
			name: "evaluateGuardrail",
			call: (*Handler).evaluateGuardrail,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/ops/guardrails/evaluate", strings.NewReader(`{}`))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler(t, nil)
			h.guardrails = nil
			w := httptest.NewRecorder()
			tc.call(h, w, tc.req())
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// enforceGuardrail block / confirm decisions
// ---------------------------------------------------------------------------

func TestEnforceGuardrailDecisions(t *testing.T) {
	t.Parallel()

	t.Run("nil guardrails allows action", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil)
		h.guardrails = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		if !h.enforceGuardrail(w, r, guardrails.Input{Action: "session.kill"}) {
			t.Fatal("nil guardrails should allow action")
		}
	})
}

func TestRegisterAddsRoutesAndShutsDown(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	guard := security.New("", nil, security.CookieSecureAuto)
	st := newTestStore(t)
	h := Register(mux, guard, st, &mockOpsControlPlane{}, events.NewHub(), "v1", "", "UTC", "", 2)
	if h == nil {
		t.Fatal("Register returned nil handler")
	}
	h.SetNotifier(nil)
	h.Shutdown(context.Background())
}
