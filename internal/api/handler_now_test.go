package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestNowReturnsHealthyEmptySnapshot(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	response := requestNow(t, h)

	if response.Confidence.State != nowConfidenceCurrent {
		t.Fatalf("confidence = %q, want current", response.Confidence.State)
	}
	if response.Posture.State != nowPostureHealthy {
		t.Fatalf("posture = %q, want healthy", response.Posture.State)
	}
	if response.Attention.Total != 0 || len(response.Attention.Visible) != 0 {
		t.Fatalf("attention = %+v, want empty", response.Attention)
	}
	for name, source := range map[string]nowSource{
		"tmux":     response.Confidence.Sources.Tmux,
		"services": response.Confidence.Sources.Services,
		"metrics":  response.Confidence.Sources.Metrics,
		"runbooks": response.Confidence.Sources.Runbooks,
	} {
		if source.Status != nowSourceCurrent {
			t.Fatalf("%s source = %+v, want current", name, source)
		}
		if _, err := time.Parse(time.RFC3339, source.ObservedAt); err != nil {
			t.Fatalf("%s observedAt = %q: %v", name, source.ObservedAt, err)
		}
	}
}

func TestNowIsolatesServiceFailureAsPartialResponse(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	h.ops = &mockOpsControlPlane{
		listServicesFn: func(context.Context) ([]opsplane.ServiceStatus, error) {
			return nil, errors.New("sensitive provider failure")
		},
		metricsSnapshotFn: func(context.Context) opsplane.MetricsSnapshot {
			return opsplane.MetricsSnapshot{
				Metrics: opsplane.HostMetrics{CPUPercent: 10},
				Posture: opsplane.MetricPosture{
					State:      opsplane.MetricPostureStateNormal,
					Severity:   opsplane.MetricPostureSeverityOK,
					Signals:    []opsplane.MetricPostureSignal{},
					ObservedAt: "2026-07-27T12:00:00Z",
				},
			}
		},
	}

	response := requestNow(t, h)
	if response.Confidence.State != nowConfidenceDegraded {
		t.Fatalf("confidence = %q, want degraded", response.Confidence.State)
	}
	if response.Posture.State != nowPostureUnknown {
		t.Fatalf("posture = %q, want unknown", response.Posture.State)
	}
	if response.Confidence.Sources.Services.Status != nowSourceUnavailable ||
		response.Confidence.Sources.Services.Message != "services_unavailable" {
		t.Fatalf("services source = %+v", response.Confidence.Sources.Services)
	}
	if response.Confidence.Sources.Tmux.Status != nowSourceCurrent ||
		response.Confidence.Sources.Metrics.Status != nowSourceCurrent ||
		response.Confidence.Sources.Runbooks.Status != nowSourceCurrent {
		t.Fatalf("healthy sources were degraded: %+v", response.Confidence.Sources)
	}
	if got, want := response.Confidence.Sources.Metrics.ObservedAt, "2026-07-27T12:00:00Z"; got != want {
		t.Fatalf("metrics observedAt = %q, want %q", got, want)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sensitive provider failure") {
		t.Fatalf("internal error leaked in response: %s", encoded)
	}
}

func TestNowReportsStaleTmuxProjection(t *testing.T) {
	t.Parallel()

	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h, st := newTestHandler(t, tm)
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertWatchtowerSession(context.Background(), store.WatchtowerSessionWrite{
		SessionName:   "dev",
		Windows:       1,
		Panes:         1,
		ActivityAt:    now,
		LastPreviewAt: now,
		UnreadPanes:   2,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}

	response := requestNow(t, h)
	if response.Confidence.Sources.Tmux.Status != nowSourceStale ||
		response.Confidence.Sources.Tmux.Message != "tmux_projection_stale" {
		t.Fatalf("tmux source = %+v, want stale projection", response.Confidence.Sources.Tmux)
	}
	if len(response.InProgress.Sessions) != 1 || response.InProgress.Sessions[0].Name != "dev" {
		t.Fatalf("sessions = %+v, want projected dev", response.InProgress.Sessions)
	}
	if response.Confidence.State != nowConfidenceDegraded {
		t.Fatalf("confidence = %q, want degraded", response.Confidence.State)
	}
	if got, want := response.Confidence.Sources.Tmux.ObservedAt, now.Format(time.RFC3339); got != want {
		t.Fatalf("tmux observedAt = %q, want %q", got, want)
	}
	if response.Posture.State != nowPostureHealthy {
		t.Fatalf("posture = %q, want healthy despite stale tmux", response.Posture.State)
	}
}

func TestNowReportsTmuxNotConfiguredAndNoServer(t *testing.T) {
	t.Parallel()

	t.Run("not configured", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, &mockTmux{
			listSessionsFn: func(context.Context) ([]tmux.Session, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindNotFound}
			},
		})
		if err := st.UpsertSession(context.Background(), "preserved", "hash", "content"); err != nil {
			t.Fatal(err)
		}
		response := requestNow(t, h)
		if response.Confidence.Sources.Tmux.Status != nowSourceNotConfigured {
			t.Fatalf("tmux source = %+v, want not_configured", response.Confidence.Sources.Tmux)
		}
		metadata, err := st.GetAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := metadata["preserved"]; !ok {
			t.Fatal("runtime failure purged persisted session metadata")
		}
	})

	t.Run("installed without server", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{
			listSessionsFn: func(context.Context) ([]tmux.Session, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindServerNotRunning}
			},
		})
		response := requestNow(t, h)
		if response.Confidence.Sources.Tmux.Status != nowSourceCurrent {
			t.Fatalf("tmux source = %+v, want current empty", response.Confidence.Sources.Tmux)
		}
		if len(response.InProgress.Sessions) != 0 {
			t.Fatalf("sessions = %+v, want empty", response.InProgress.Sessions)
		}
	})
}

func TestNowUnavailableOnlyForMissingStructuralDependency(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	h.ops = nil
	w := httptest.NewRecorder()
	h.now(w, httptest.NewRequest(http.MethodGet, "/api/now", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	body := jsonBody(t, w)
	errBody := body["error"].(map[string]any)
	if errBody["code"] != "NOW_UNAVAILABLE" {
		t.Fatalf("error = %+v", errBody)
	}
}

func TestRunNowServiceRunbookRevalidatesRecoveredService(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
	h.ops = &mockOpsControlPlane{
		listServicesFn: func(context.Context) ([]opsplane.ServiceStatus, error) {
			return []opsplane.ServiceStatus{{Name: "database", ActiveState: "active"}}, nil
		},
	}
	if _, err := st.InsertOpsRunbook(context.Background(), store.OpsRunbookWrite{
		ID:            "database-recovery",
		Name:          "Database recovery",
		Enabled:       true,
		TargetService: "database",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "recover", Command: "true"}},
	}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/now/services/database/runbook", strings.NewReader(`{}`))
	r.SetPathValue("service", "database")
	h.runNowServiceRunbook(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	if body["error"].(map[string]any)["code"] != "NOW_SERVICE_NOT_FAILED" {
		t.Fatalf("body = %s", w.Body.String())
	}
	runs, err := st.ListOpsRunbookRuns(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %+v, want none", runs)
	}
}

func TestRunNowServiceRunbookCreatesCanonicalJobAndOnlyJobEvents(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
	h.ops = &mockOpsControlPlane{
		listServicesFn: func(context.Context) ([]opsplane.ServiceStatus, error) {
			return []opsplane.ServiceStatus{{Name: "database", ActiveState: "failed"}}, nil
		},
	}
	if _, err := st.InsertOpsRunbook(context.Background(), store.OpsRunbookWrite{
		ID:            "database-recovery",
		Name:          "Database recovery",
		Enabled:       true,
		TargetService: "database",
		Parameters: []store.RunbookParameter{{
			Name: "MODE", Type: "select", Required: true, Options: []string{"safe"},
		}},
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "recover", Command: "true"}},
	}); err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub()
	eventChannel, unsubscribe := hub.Subscribe(8)
	t.Cleanup(unsubscribe)
	h.events = hub

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		http.MethodPost,
		"/api/now/services/database/runbook",
		strings.NewReader(`{"parameters":{"MODE":"safe"}}`),
	)
	r.SetPathValue("service", "database")
	h.runNowServiceRunbook(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	var payload struct {
		Data struct {
			Job store.OpsRunbookRun `json:"job"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	h.runbooks.WaitIdle()
	persisted, err := st.GetOpsRunbookRun(context.Background(), payload.Data.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Source != store.OpsRunbookRunSourceNow ||
		persisted.TargetKind != store.OpsRunbookRunTargetService ||
		persisted.TargetName != "database" {
		t.Fatalf("job context = (%q, %q, %q)", persisted.Source, persisted.TargetKind, persisted.TargetName)
	}
	if persisted.ParametersUsed["MODE"] != "safe" {
		t.Fatalf("parameters = %+v", persisted.ParametersUsed)
	}

	eventsSeen := 0
	for {
		select {
		case event := <-eventChannel:
			eventsSeen++
			if event.Type != events.TypeOpsJob {
				t.Fatalf("event type = %q, want only %q", event.Type, events.TypeOpsJob)
			}
		default:
			if eventsSeen == 0 {
				t.Fatal("no ops.job.updated event was emitted")
			}
			return
		}
	}
}

func TestNowRoutesRequireAuthenticationAndRestrictMethods(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	guard := security.New("secret", nil, security.CookieSecureAuto)
	st := newTestStore(t)
	h := Register(
		mux,
		guard,
		st,
		&mockOpsControlPlane{},
		events.NewHub(),
		"test",
		"",
		"UTC",
		"en",
		nil,
		1,
	)
	h.tmux = &mockTmux{}
	t.Cleanup(func() { h.Shutdown(context.Background()) })

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/now", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized GET = %d, want 401", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/now", nil)
	cookieRecorder := httptest.NewRecorder()
	guard.SetAuthCookie(cookieRecorder, authorizedRequest)
	for _, cookie := range cookieRecorder.Result().Cookies() {
		authorizedRequest.AddCookie(cookie)
	}
	authorized := httptest.NewRecorder()
	mux.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized GET = %d, want 200; body=%s", authorized.Code, authorized.Body.String())
	}

	wrongMethodRequest := httptest.NewRequest(http.MethodPut, "/api/now/services/database/runbook", nil)
	for _, cookie := range cookieRecorder.Result().Cookies() {
		wrongMethodRequest.AddCookie(cookie)
	}
	wrongMethod := httptest.NewRecorder()
	mux.ServeHTTP(wrongMethod, wrongMethodRequest)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method = %d, want 405", wrongMethod.Code)
	}
}

func requestNow(t *testing.T, h *Handler) nowResponse {
	t.Helper()

	w := httptest.NewRecorder()
	h.now(w, httptest.NewRequest(http.MethodGet, "/api/now", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Data struct {
			Now nowResponse `json:"now"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode Now response: %v", err)
	}
	return payload.Data.Now
}
