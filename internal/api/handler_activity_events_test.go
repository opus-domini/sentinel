package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/guardrails"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

// TestAlertLifecycleTimelineEvents verifies that the alert lifecycle
// (create -> ack -> resolve -> delete) produces the expected ops timeline
// entries via the orchestrator.
func TestAlertLifecycleTimelineEvents(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// 1. Create an alert via a failed service action, which should record
	//    both a service.action event and an alert.created event.
	serviceStatus := opsplane.ServiceStatus{
		Name:        "my-svc",
		DisplayName: "My Service",
		Unit:        "my-svc.service",
		Manager:     "systemd",
		Scope:       "user",
		ActiveState: "failed",
	}
	_, _, firedAlerts, err := h.orch.RecordServiceAction(ctx, serviceStatus, "restart", now)
	if err != nil {
		t.Fatalf("RecordServiceAction: %v", err)
	}
	if len(firedAlerts) != 1 {
		t.Fatalf("expected 1 fired alert, got %d", len(firedAlerts))
	}

	// Verify we have both service.action and alert.created events.
	result, err := st.SearchActivityEvents(ctx, activity.Query{Limit: 50})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}

	eventTypes := map[string]bool{}
	for _, e := range result.Events {
		eventTypes[e.EventType] = true
	}
	if !eventTypes["service.action"] {
		t.Error("missing service.action event")
	}
	if !eventTypes["alert.created"] {
		t.Error("missing alert.created event")
	}

	// 2. Ack the alert — should record alert.acked event.
	alert := firedAlerts[0]
	_, _, acked, err := h.orch.AckAlert(ctx, alert.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("AckAlert: %v", err)
	}
	if !acked {
		t.Fatal("expected ack to record a timeline event")
	}

	result, err = st.SearchActivityEvents(ctx, activity.Query{Source: "alert", Limit: 50})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	eventTypes = map[string]bool{}
	for _, e := range result.Events {
		eventTypes[e.EventType] = true
	}
	if !eventTypes["alert.acked"] {
		t.Error("missing alert.acked event after ack")
	}

	// 3. Resolve the alert — verify alert.resolved via the handler-level
	//    ResolveAlert on the store, then check that the orchestrator's
	//    RecordAlertResolved method records the event.
	resolvedAlert, resolveErr := st.ResolveAlert(ctx, alert.DedupeKey, now.Add(2*time.Minute))
	if resolveErr != nil {
		t.Fatalf("ResolveAlert: %v", resolveErr)
	}
	h.orch.RecordAlertResolved(ctx, resolvedAlert, now.Add(2*time.Minute))

	result, err = st.SearchActivityEvents(ctx, activity.Query{Source: "alert", Limit: 50})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	eventTypes = map[string]bool{}
	for _, e := range result.Events {
		eventTypes[e.EventType] = true
	}
	if !eventTypes["alert.resolved"] {
		t.Error("missing alert.resolved event after resolve")
	}

	// 4. Delete the alert via the handler — should record alert.deleted.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", fmt.Sprintf("/api/ops/alerts/%d", resolvedAlert.ID), nil)
	r.SetPathValue("alert", fmt.Sprintf("%d", resolvedAlert.ID))
	h.deleteOpsAlert(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("deleteOpsAlert status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	result, err = st.SearchActivityEvents(ctx, activity.Query{Source: "alert", Limit: 50})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	eventTypes = map[string]bool{}
	for _, e := range result.Events {
		eventTypes[e.EventType] = true
	}
	if !eventTypes["alert.deleted"] {
		t.Error("missing alert.deleted event after delete")
	}
}

// TestAlertLifecycleTableDriven uses table-driven sub-tests to verify each
// alert lifecycle event type individually.
func TestAlertLifecycleTableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T, h *Handler, st *store.Store, ctx context.Context) int64 // returns alert ID
		action        func(t *testing.T, h *Handler, st *store.Store, ctx context.Context, alertID int64)
		wantEventType string
	}{
		{
			name: "alert.created via failed service",
			setup: func(_ *testing.T, _ *Handler, _ *store.Store, _ context.Context) int64 {
				return 0 // no pre-existing alert needed
			},
			action: func(t *testing.T, h *Handler, _ *store.Store, ctx context.Context, _ int64) {
				t.Helper()
				status := opsplane.ServiceStatus{
					Name:        "test-svc",
					DisplayName: "Test Svc",
					Unit:        "test-svc.service",
					Manager:     "systemd",
					ActiveState: "failed",
				}
				_, _, _, err := h.orch.RecordServiceAction(ctx, status, "start", time.Now().UTC())
				if err != nil {
					t.Fatalf("RecordServiceAction: %v", err)
				}
			},
			wantEventType: "alert.created",
		},
		{
			name: "alert.acked",
			setup: func(t *testing.T, _ *Handler, st *store.Store, ctx context.Context) int64 {
				t.Helper()
				a, err := st.UpsertAlert(ctx, alerts.AlertWrite{
					DedupeKey: "ack:test",
					Source:    "test",
					Resource:  "test-resource",
					Title:     "Test alert for ack",
					Severity:  "warn",
					CreatedAt: time.Now().UTC(),
				})
				if err != nil {
					t.Fatalf("UpsertAlert: %v", err)
				}
				return a.ID
			},
			action: func(t *testing.T, h *Handler, _ *store.Store, ctx context.Context, alertID int64) {
				t.Helper()
				_, _, acked, err := h.orch.AckAlert(ctx, alertID, time.Now().UTC())
				if err != nil {
					t.Fatalf("AckAlert: %v", err)
				}
				if !acked {
					t.Fatal("expected ack to record timeline event")
				}
			},
			wantEventType: "alert.acked",
		},
		{
			name: "alert.resolved",
			setup: func(t *testing.T, _ *Handler, st *store.Store, ctx context.Context) int64 {
				t.Helper()
				a, err := st.UpsertAlert(ctx, alerts.AlertWrite{
					DedupeKey: "resolve:test",
					Source:    "test",
					Resource:  "test-resource",
					Title:     "Test alert for resolve",
					Severity:  "error",
					CreatedAt: time.Now().UTC(),
				})
				if err != nil {
					t.Fatalf("UpsertAlert: %v", err)
				}
				return a.ID
			},
			action: func(t *testing.T, h *Handler, st *store.Store, ctx context.Context, _ int64) {
				t.Helper()
				resolved, err := st.ResolveAlert(ctx, "resolve:test", time.Now().UTC())
				if err != nil {
					t.Fatalf("ResolveAlert: %v", err)
				}
				h.orch.RecordAlertResolved(ctx, resolved, time.Now().UTC())
			},
			wantEventType: "alert.resolved",
		},
		{
			name: "alert.deleted",
			setup: func(t *testing.T, _ *Handler, st *store.Store, ctx context.Context) int64 {
				t.Helper()
				a, err := st.UpsertAlert(ctx, alerts.AlertWrite{
					DedupeKey: "del:test",
					Source:    "test",
					Resource:  "test-resource",
					Title:     "Test alert for delete",
					Severity:  "error",
					CreatedAt: time.Now().UTC(),
				})
				if err != nil {
					t.Fatalf("UpsertAlert: %v", err)
				}
				// Resolve it first — delete requires resolved status.
				if _, err := st.ResolveAlert(ctx, "del:test", time.Now().UTC()); err != nil {
					t.Fatalf("ResolveAlert: %v", err)
				}
				return a.ID
			},
			action: func(t *testing.T, h *Handler, _ *store.Store, _ context.Context, alertID int64) {
				t.Helper()
				w := httptest.NewRecorder()
				r := httptest.NewRequest("DELETE", fmt.Sprintf("/api/ops/alerts/%d", alertID), nil)
				r.SetPathValue("alert", fmt.Sprintf("%d", alertID))
				h.deleteOpsAlert(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("deleteOpsAlert status = %d, want 200", w.Code)
				}
			},
			wantEventType: "alert.deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, st := newTestHandler(t, nil, nil)
			ctx := context.Background()

			alertID := tt.setup(t, h, st, ctx)
			tt.action(t, h, st, ctx, alertID)

			result, err := st.SearchActivityEvents(ctx, activity.Query{Limit: 50})
			if err != nil {
				t.Fatalf("SearchActivityEvents: %v", err)
			}

			found := false
			for _, e := range result.Events {
				if e.EventType == tt.wantEventType {
					found = true
					break
				}
			}
			if !found {
				types := make([]string, len(result.Events))
				for i, e := range result.Events {
					types[i] = e.EventType
				}
				t.Fatalf("expected event type %q, found: %v", tt.wantEventType, types)
			}
		})
	}
}

// TestGuardrailBlockedTimelineEvent verifies that a guardrail block
// records a guardrail.blocked activity event.
func TestGuardrailBlockedTimelineEvent(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)
	ctx := context.Background()

	// Create a blocking rule that matches session.kill.
	if err := st.UpsertGuardrailRule(ctx, store.GuardrailRuleWrite{
		ID:       "block-kill",
		Name:     "Block kill",
		Scope:    "action",
		Pattern:  `^session\.kill$`,
		Mode:     "block",
		Severity: "error",
		Message:  "killing sessions is blocked",
		Enabled:  true,
		Priority: 1,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/test", nil)
	allowed := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.kill",
		SessionName: "dev",
	})
	if allowed {
		t.Fatal("expected guardrail to block the action")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}

	// Verify a guardrail.blocked event was recorded.
	result, err := st.SearchActivityEvents(ctx, activity.Query{Source: "guardrail", Limit: 50})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	found := false
	for _, e := range result.Events {
		if e.EventType == "guardrail.blocked" {
			found = true
			if !strings.Contains(e.Message, "session.kill") {
				t.Errorf("message = %q, expected to contain session.kill", e.Message)
			}
			if e.Resource != "dev" {
				t.Errorf("resource = %q, want dev", e.Resource)
			}
		}
	}
	if !found {
		t.Fatal("missing guardrail.blocked event")
	}
}

// TestScheduleLifecycleTimelineEvents verifies that schedule create,
// trigger, and delete produce timeline events.
func TestScheduleLifecycleTimelineEvents(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()

	// Get a runbook ID for the schedule.
	runbooks, err := st.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatal("expected at least one seeded runbook")
	}
	runbookID := runbooks[0].ID

	t.Run("schedule.created", func(t *testing.T) {
		// Create a "once" schedule with a future runAt.
		futureAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
		body := fmt.Sprintf(`{"runbookId":%q,"name":"test-schedule","scheduleType":"once","runAt":%q,"enabled":true}`, runbookID, futureAt)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/schedules", strings.NewReader(body))
		h.createSchedule(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("createSchedule status = %d, want 201, body=%s", w.Code, w.Body.String())
		}

		result, err := st.SearchActivityEvents(ctx, activity.Query{Source: "schedule", Limit: 50})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		found := false
		for _, e := range result.Events {
			if e.EventType == "schedule.created" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing schedule.created event")
		}
	})

	t.Run("schedule.triggered", func(t *testing.T) {
		// Create a "once" schedule, then trigger it.
		futureAt := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
		sched, insertErr := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID:    runbookID,
			Name:         "trigger-test",
			ScheduleType: "once",
			RunAt:        futureAt,
			Enabled:      true,
			NextRunAt:    futureAt,
		})
		if insertErr != nil {
			t.Fatalf("InsertOpsSchedule: %v", insertErr)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/schedules/"+sched.ID+"/trigger", nil)
		r.SetPathValue("schedule", sched.ID)
		h.triggerSchedule(w, r)
		if w.Code != http.StatusAccepted {
			t.Fatalf("triggerSchedule status = %d, want 202, body=%s", w.Code, w.Body.String())
		}

		// Wait for async runbook to finish.
		h.wg.Wait()

		result, err := st.SearchActivityEvents(ctx, activity.Query{Source: "schedule", Limit: 50})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		found := false
		for _, e := range result.Events {
			if e.EventType == "schedule.triggered" {
				found = true
			}
		}
		if !found {
			types := make([]string, len(result.Events))
			for i, e := range result.Events {
				types[i] = e.EventType
			}
			t.Fatalf("missing schedule.triggered event; found: %v", types)
		}
	})

	t.Run("schedule.deleted", func(t *testing.T) {
		// Create a schedule, then delete it.
		futureAt := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
		sched, insertErr := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID:    runbookID,
			Name:         "delete-test",
			ScheduleType: "once",
			RunAt:        futureAt,
			Enabled:      true,
			NextRunAt:    futureAt,
		})
		if insertErr != nil {
			t.Fatalf("InsertOpsSchedule: %v", insertErr)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/schedules/"+sched.ID, nil)
		r.SetPathValue("schedule", sched.ID)
		h.deleteSchedule(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("deleteSchedule status = %d, want 200", w.Code)
		}

		result, err := st.SearchActivityEvents(ctx, activity.Query{Source: "schedule", Limit: 50})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		found := false
		for _, e := range result.Events {
			if e.EventType == "schedule.deleted" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing schedule.deleted event")
		}
	})
}

// TestOrchestratorNilGuards ensures orchestrator methods do not panic when
// the orchestrator or its repo is nil.
func TestOrchestratorNilGuards(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "RecordAlertCreated nil orch",
			fn: func() {
				var o *opsOrchestrator
				o.RecordAlertCreated(ctx, alerts.Alert{}, now)
			},
		},
		{
			name: "RecordAlertResolved nil repo",
			fn: func() {
				o := &opsOrchestrator{}
				o.RecordAlertResolved(ctx, alerts.Alert{}, now)
			},
		},
		{
			name: "RecordAlertDeleted nil orch",
			fn: func() {
				var o *opsOrchestrator
				o.RecordAlertDeleted(ctx, 1, now)
			},
		},
		{
			name: "RecordGuardrailBlocked nil repo",
			fn: func() {
				o := &opsOrchestrator{}
				o.RecordGuardrailBlocked(ctx, "session.kill", "dev", "%0", "blocked", now)
			},
		},
		{
			name: "RecordScheduleCreated nil orch",
			fn: func() {
				var o *opsOrchestrator
				o.RecordScheduleCreated(ctx, store.OpsSchedule{}, now)
			},
		},
		{
			name: "RecordScheduleTriggered nil repo",
			fn: func() {
				o := &opsOrchestrator{}
				o.RecordScheduleTriggered(ctx, "s1", "rb1", "j1", now)
			},
		},
		{
			name: "RecordScheduleDeleted nil orch",
			fn: func() {
				var o *opsOrchestrator
				o.RecordScheduleDeleted(ctx, "s1", now)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Must not panic.
			tt.fn()
		})
	}
}
