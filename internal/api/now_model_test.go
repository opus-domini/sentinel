package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

func TestBuildNowModelFairlyRepresentsEveryAttentionCategory(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	active := make([]store.OpsRunbookRun, 0, 5)
	for i := range 5 {
		active = append(active, nowTestRun(
			"approval-"+string(rune('a'+i)),
			store.OpsRunbookStatusWaitingApproval,
			base.Add(time.Duration(i)*time.Minute),
		))
	}
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Services: []opsplane.ServiceStatus{{
			Name: "sentinel", ActiveState: "failed",
		}},
		Metrics: pressureNowPosture(
			opsplane.MetricPostureSeverityWarning,
			base,
		),
		ActiveRuns: active,
		Sources:    healthyNowSources(),
	})

	if model.Attention.Total != 7 {
		t.Fatalf("attention total = %d, want 7", model.Attention.Total)
	}
	if len(model.Attention.Visible) != nowAttentionLimit {
		t.Fatalf("visible = %d, want %d", len(model.Attention.Visible), nowAttentionLimit)
	}
	gotTypes := make([]string, 0, len(model.Attention.Visible))
	for _, item := range model.Attention.Visible {
		gotTypes = append(gotTypes, item.Type)
	}
	wantTypes := []string{
		nowAttentionServiceFailed,
		nowAttentionRunbookApproval,
		nowAttentionMetricsPressure,
		nowAttentionRunbookApproval,
		nowAttentionRunbookApproval,
	}
	if strings.Join(gotTypes, ",") != strings.Join(wantTypes, ",") {
		t.Fatalf("attention order = %v, want %v", gotTypes, wantTypes)
	}
	if got := model.Attention.Visible[1].Run.RunID; got != "approval-a" {
		t.Fatalf("oldest approval = %q, want approval-a", got)
	}
	wantOverflow := nowAttentionOverflow{Approvals: 2}
	if model.Attention.Overflow != wantOverflow {
		t.Fatalf("overflow = %+v, want %+v", model.Attention.Overflow, wantOverflow)
	}
	metrics := model.Attention.Visible[2]
	if metrics.ObservedAt != base.Format(time.RFC3339) {
		t.Fatalf("metrics observedAt = %q", metrics.ObservedAt)
	}
}

func TestBuildNowModelCriticalMetricsPrecedesApprovalAndRemainingServices(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Services: []opsplane.ServiceStatus{
			{Name: "delta", ActiveState: "failed"},
			{Name: "charlie", ActiveState: "failed"},
			{Name: "bravo", ActiveState: "failed"},
			{Name: "alpha", ActiveState: "failed"},
		},
		Metrics: pressureNowPosture(
			opsplane.MetricPostureSeverityCritical,
			base,
		),
		ActiveRuns: []store.OpsRunbookRun{
			nowTestRun("approval-c", store.OpsRunbookStatusWaitingApproval, base.Add(2*time.Minute)),
			nowTestRun("approval-a", store.OpsRunbookStatusWaitingApproval, base),
			nowTestRun("approval-b", store.OpsRunbookStatusWaitingApproval, base.Add(time.Minute)),
		},
		Sources: healthyNowSources(),
	})

	got := make([]string, 0, len(model.Attention.Visible))
	for _, item := range model.Attention.Visible {
		switch item.Type {
		case nowAttentionServiceFailed:
			got = append(got, "service:"+item.Service.Name)
		case nowAttentionRunbookApproval:
			got = append(got, "approval:"+item.Run.RunID)
		default:
			got = append(got, "metrics:"+item.Severity)
		}
	}
	want := []string{
		"service:alpha",
		"metrics:critical",
		"approval:approval-a",
		"service:bravo",
		"service:charlie",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("attention order = %v, want %v", got, want)
	}
	if model.Attention.Overflow != (nowAttentionOverflow{Approvals: 2, Services: 1}) {
		t.Fatalf("overflow = %+v", model.Attention.Overflow)
	}
}

func TestBuildNowModelDoesNotAdmitTerminalFailure(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Metrics:     normalNowPosture(base),
		ActiveRuns: []store.OpsRunbookRun{
			nowTestRun("historical-failure", store.OpsRunbookStatusFailed, base),
		},
		Sources: healthyNowSources(),
	})

	if model.Attention.Total != 0 || len(model.Attention.Visible) != 0 {
		t.Fatalf("historical failure created attention: %+v", model.Attention)
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "runbook_failed") ||
		strings.Contains(string(encoded), `"failure"`) {
		t.Fatalf("removed historical fields leaked: %s", encoded)
	}
}

func TestBuildNowModelSeparatesConfidenceAndPosture(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name           string
		sources        nowSources
		services       []opsplane.ServiceStatus
		metrics        opsplane.MetricPosture
		wantConfidence string
		wantPosture    string
	}{
		{
			name: "healthy current", sources: healthyNowSources(),
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceCurrent, wantPosture: nowPostureHealthy,
		},
		{
			name: "at risk current", sources: healthyNowSources(),
			services:       []opsplane.ServiceStatus{{Name: "sentinel", ActiveState: "failed"}},
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceCurrent, wantPosture: nowPostureAtRisk,
		},
		{
			name: "tmux stale keeps host posture", sources: func() nowSources {
				sources := healthyNowSources()
				sources.Tmux.Status = nowSourceStale
				return sources
			}(),
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceDegraded, wantPosture: nowPostureHealthy,
		},
		{
			name: "runbooks unavailable keeps host posture", sources: func() nowSources {
				sources := healthyNowSources()
				sources.Runbooks.Status = nowSourceUnavailable
				return sources
			}(),
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceDegraded, wantPosture: nowPostureHealthy,
		},
		{
			name: "services stale makes posture unknown", sources: func() nowSources {
				sources := healthyNowSources()
				sources.Services.Status = nowSourceStale
				return sources
			}(),
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceDegraded, wantPosture: nowPostureUnknown,
		},
		{
			name: "metrics unavailable makes posture unknown", sources: func() nowSources {
				sources := healthyNowSources()
				sources.Metrics.Status = nowSourceUnavailable
				return sources
			}(),
			metrics:        normalNowPosture(base),
			wantConfidence: nowConfidenceDegraded, wantPosture: nowPostureUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := buildNowModel(nowModelInput{
				GeneratedAt: base,
				Services:    tt.services,
				Metrics:     tt.metrics,
				Sources:     tt.sources,
			})
			if model.Confidence.State != tt.wantConfidence ||
				model.Posture.State != tt.wantPosture {
				t.Fatalf(
					"confidence/posture = %q/%q, want %q/%q",
					model.Confidence.State,
					model.Posture.State,
					tt.wantConfidence,
					tt.wantPosture,
				)
			}
		})
	}
}

func TestBuildNowModelUsesUnreadOnlySessionsAndDeterministicOrder(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC)
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Metrics:     normalNowPosture(base),
		Sessions: []enrichedSession{
			{Name: "pinned-quiet", ActivityAt: base.Add(10 * time.Minute).Format(time.RFC3339)},
			{Name: "zeta", User: "bob", UnreadPanes: 1, ActivityAt: base.Add(3 * time.Minute).Format(time.RFC3339)},
			{Name: "beta", User: "amy", UnreadWindows: 1, ActivityAt: base.Add(3 * time.Minute).Format(time.RFC3339)},
			{Name: "alpha", User: "amy", UnreadPanes: 1, ActivityAt: base.Add(3 * time.Minute).Format(time.RFC3339)},
			{Name: "newest", UnreadPanes: 1, ActivityAt: base.Add(4 * time.Minute).Format(time.RFC3339)},
		},
		SessionPresets: []store.SessionPreset{{Name: "pinned-quiet"}, {Name: "alpha"}},
		Sources:        healthyNowSources(),
	})

	got := make([]string, 0, len(model.InProgress.Sessions))
	for _, session := range model.InProgress.Sessions {
		got = append(got, session.Name)
	}
	if strings.Join(got, ",") != "newest,alpha,beta" {
		t.Fatalf("sessions = %v", got)
	}
	if !model.InProgress.Sessions[1].Pinned {
		t.Fatalf("pin metadata was not preserved: %+v", model.InProgress.Sessions[1])
	}
	for _, session := range model.InProgress.Sessions {
		if session.Name == "pinned-quiet" {
			t.Fatal("quiet pinned session qualified as in progress")
		}
	}
}

func TestBuildNowModelBuildsBoundedInProgressRuns(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	active := []store.OpsRunbookRun{
		nowTestRun("waiting", store.OpsRunbookStatusWaitingApproval, base.Add(5*time.Minute)),
		nowTestRun("queued-old", store.OpsRunbookStatusQueued, base),
		nowTestRun("running-new", store.OpsRunbookStatusRunning, base.Add(4*time.Minute)),
		nowTestRun("queued-new", store.OpsRunbookStatusQueued, base.Add(3*time.Minute)),
		nowTestRun("running-old", store.OpsRunbookStatusRunning, base.Add(2*time.Minute)),
	}
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Metrics:     normalNowPosture(base),
		ActiveRuns:  active,
		Sources:     healthyNowSources(),
	})

	runIDs := []string{
		model.InProgress.Runs[0].ID,
		model.InProgress.Runs[1].ID,
		model.InProgress.Runs[2].ID,
	}
	if strings.Join(runIDs, ",") != "running-new,queued-new,running-old" {
		t.Fatalf("in-progress runs = %v", runIDs)
	}
}

func nowTestRun(id, status string, at time.Time) store.OpsRunbookRun {
	return store.OpsRunbookRun{
		ID:          id,
		RunbookID:   "rb-" + id,
		RunbookName: "Runbook " + id,
		Status:      status,
		TotalSteps:  2,
		Source:      store.OpsRunbookRunSourceRunbooks,
		CreatedAt:   at.Format(time.RFC3339),
	}
}

func normalNowPosture(observedAt time.Time) opsplane.MetricPosture {
	return opsplane.MetricPosture{
		State:      opsplane.MetricPostureStateNormal,
		Severity:   opsplane.MetricPostureSeverityOK,
		Signals:    []opsplane.MetricPostureSignal{},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
	}
}

func pressureNowPosture(severity string, observedAt time.Time) opsplane.MetricPosture {
	signal := opsplane.MetricPostureSignal{
		Name:     "cpu",
		Severity: severity,
		Value:    85,
		Since:    observedAt.Add(-10 * time.Second).UTC().Format(time.RFC3339),
	}
	posture := opsplane.MetricPosture{
		State:      opsplane.MetricPostureStatePressure,
		Severity:   severity,
		Signals:    []opsplane.MetricPostureSignal{signal},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
	}
	if severity == opsplane.MetricPostureSeverityCritical {
		posture.CriticalCount = 1
	} else {
		posture.WarningCount = 1
	}
	return posture
}

func healthyNowSources() nowSources {
	observedAt := "2026-07-27T10:00:00Z"
	return nowSources{
		Tmux:     nowSource{Status: nowSourceCurrent, ObservedAt: observedAt},
		Services: nowSource{Status: nowSourceCurrent, ObservedAt: observedAt},
		Metrics:  nowSource{Status: nowSourceCurrent, ObservedAt: observedAt},
		Runbooks: nowSource{Status: nowSourceCurrent, ObservedAt: observedAt},
	}
}
