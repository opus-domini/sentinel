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
	"github.com/opus-domini/sentinel/internal/events"
)

func TestBulkAckOpsAlerts(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	ctx := context.Background()

	first, err := st.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "bulk:first",
		Source:    "test",
		Resource:  "svc-a",
		Title:     "First",
		Severity:  "warn",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertAlert(first): %v", err)
	}
	second, err := st.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "bulk:second",
		Source:    "test",
		Resource:  "svc-b",
		Title:     "Second",
		Severity:  "error",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertAlert(second): %v", err)
	}

	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"ids":[%d,%d]}`, first.ID, second.ID)
	r := httptest.NewRequest(http.MethodPost, "/api/ops/alerts/bulk-ack", strings.NewReader(body))
	h.bulkAckOpsAlerts(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("bulkAckOpsAlerts status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	list, err := st.ListAlerts(ctx, 10, alerts.StatusAcked)
	if err != nil {
		t.Fatalf("ListAlerts(acked): %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("acked alerts len = %d, want 2", len(list))
	}
	activity, err := st.SearchActivityEvents(ctx, activityQueryForTest("alert.acked"))
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	if len(activity.Events) != 2 {
		t.Fatalf("activity events len = %d, want 2", len(activity.Events))
	}
}

func TestBulkAckOpsAlertsValidatesIDs(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/alerts/bulk-ack", strings.NewReader(`{"ids":[]}`))
	h.bulkAckOpsAlerts(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("bulkAckOpsAlerts empty ids status = %d, want 400", w.Code)
	}
}

func activityQueryForTest(eventType string) activity.Query {
	return activity.Query{
		Query: eventType,
		Limit: 10,
	}
}
