package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

func TestBuildNowModelPrioritizesAttentionAndCountsOverflow(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	active := make([]store.OpsRunbookRun, 0, 6)
	for i := range 6 {
		active = append(active, nowTestRun("approval-"+string(rune('a'+i)), store.OpsRunbookStatusWaitingApproval, base.Add(time.Duration(i)*time.Minute)))
	}
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Services: []opsplane.ServiceStatus{{
			Name: "sentinel", ActiveState: "failed",
		}},
		Metrics: opsplane.MetricPosture{
			State:    opsplane.MetricPostureStatePressure,
			Severity: opsplane.MetricPostureSeverityWarning,
			Signals:  []opsplane.MetricPostureSignal{{Name: "cpu", Severity: "warning", Value: 82}},
		},
		ActiveRuns: active,
		LatestTerminalRuns: []store.OpsRunbookRun{
			nowTestRun("runbook-failed", store.OpsRunbookStatusFailed, base.Add(10*time.Minute)),
		},
		Sources: healthyNowSources(),
	})

	if model.Attention.Total != 9 {
		t.Fatalf("attention total = %d, want 9", model.Attention.Total)
	}
	if len(model.Attention.Visible) != nowAttentionLimit {
		t.Fatalf("visible = %d, want %d", len(model.Attention.Visible), nowAttentionLimit)
	}
	for index, item := range model.Attention.Visible {
		if item.Type != nowAttentionRunbookApproval {
			t.Fatalf("visible[%d].type = %q, want approval", index, item.Type)
		}
		wantID := "approval-" + string(rune('a'+index))
		if item.Run == nil || item.Run.RunID != wantID {
			t.Fatalf("visible[%d].run = %#v, want %q", index, item.Run, wantID)
		}
	}
	wantOverflow := (nowAttentionOverflow{Approvals: 1, Services: 1, Runbooks: 1, Metrics: 1})
	if model.Attention.Overflow != wantOverflow {
		t.Fatalf("overflow = %+v, want %+v", model.Attention.Overflow, wantOverflow)
	}
	encoded, err := json.Marshal(model.Attention)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "score") {
		t.Fatalf("attention exposes a score: %s", encoded)
	}
}

func TestBuildNowModelUsesAllFourAttentionPriorities(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Services: []opsplane.ServiceStatus{{
			Name: "sentinel", DisplayName: "Sentinel", ActiveState: "failed",
		}},
		Metrics: opsplane.MetricPosture{
			State:    opsplane.MetricPostureStatePressure,
			Severity: opsplane.MetricPostureSeverityCritical,
			Signals:  []opsplane.MetricPostureSignal{{Name: "memory", Severity: "critical", Value: 95}},
		},
		ActiveRuns: []store.OpsRunbookRun{
			nowTestRun("approval", store.OpsRunbookStatusWaitingApproval, base),
		},
		LatestTerminalRuns: []store.OpsRunbookRun{
			nowTestRun("failure", store.OpsRunbookStatusFailed, base.Add(time.Minute)),
		},
		Sources: healthyNowSources(),
	})

	got := make([]string, 0, len(model.Attention.Visible))
	for _, item := range model.Attention.Visible {
		got = append(got, item.Type)
	}
	want := []string{
		nowAttentionRunbookApproval,
		nowAttentionServiceFailed,
		nowAttentionRunbookFailed,
		nowAttentionMetricsPressure,
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("attention order = %v, want %v", got, want)
	}
	if model.Reliability.State != nowReliabilityAttention {
		t.Fatalf("reliability = %q, want attention", model.Reliability.State)
	}
}

func TestBuildNowModelDeduplicatesFailedRunIntoFailedService(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	failure := nowTestRun("failure", store.OpsRunbookStatusFailed, base)
	failure.TargetKind = store.OpsRunbookRunTargetService
	failure.TargetName = "sentinel"
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Services: []opsplane.ServiceStatus{{
			Name: "sentinel", ActiveState: "failed",
		}},
		Runbooks: []store.OpsRunbook{{
			ID: "rb", Name: "Recover Sentinel", Enabled: true, TargetService: "sentinel",
		}},
		Metrics:            normalNowPosture(),
		LatestTerminalRuns: []store.OpsRunbookRun{failure},
		Sources:            healthyNowSources(),
	})

	if model.Attention.Total != 1 {
		t.Fatalf("attention total = %d, want 1", model.Attention.Total)
	}
	item := model.Attention.Visible[0]
	if item.Type != nowAttentionServiceFailed || item.Failure == nil || item.Failure.RunID != failure.ID {
		t.Fatalf("deduplicated item = %#v", item)
	}
	if item.Runbook == nil || item.Runbook.ID != "rb" {
		t.Fatalf("recommended runbook = %#v", item.Runbook)
	}
}

func TestBuildNowModelBuildsBoundedInProgressLists(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	active := []store.OpsRunbookRun{
		nowTestRun("waiting", store.OpsRunbookStatusWaitingApproval, base.Add(5*time.Minute)),
		nowTestRun("queued-old", store.OpsRunbookStatusQueued, base),
		nowTestRun("running-new", store.OpsRunbookStatusRunning, base.Add(4*time.Minute)),
		nowTestRun("queued-new", store.OpsRunbookStatusQueued, base.Add(3*time.Minute)),
		nowTestRun("running-old", store.OpsRunbookStatusRunning, base.Add(2*time.Minute)),
	}
	sessions := []enrichedSession{
		{Name: "quiet", ActivityAt: base.Format(time.RFC3339)},
		{Name: "preset-two", UnreadPanes: 1, ActivityAt: base.Add(4 * time.Minute).Format(time.RFC3339)},
		{Name: "preset-one", UnreadPanes: 1, ActivityAt: base.Add(3 * time.Minute).Format(time.RFC3339)},
		{Name: "unread", UnreadPanes: 2, ActivityAt: base.Add(time.Minute).Format(time.RFC3339)},
		{Name: "window", UnreadWindows: 1, ActivityAt: base.Add(5 * time.Minute).Format(time.RFC3339)},
	}
	model := buildNowModel(nowModelInput{
		GeneratedAt: base,
		Metrics:     normalNowPosture(),
		ActiveRuns:  active,
		Sessions:    sessions,
		SessionPresets: []store.SessionPreset{
			{Name: "preset-one", SortOrder: 1},
			{Name: "preset-two", SortOrder: 2},
			{Name: "quiet", SortOrder: 3},
		},
		Sources: healthyNowSources(),
	})

	runIDs := []string{
		model.InProgress.Runs[0].ID,
		model.InProgress.Runs[1].ID,
		model.InProgress.Runs[2].ID,
	}
	if strings.Join(runIDs, ",") != "running-new,queued-new,running-old" {
		t.Fatalf("in-progress runs = %v", runIDs)
	}
	sessionNames := []string{
		model.InProgress.Sessions[0].Name,
		model.InProgress.Sessions[1].Name,
		model.InProgress.Sessions[2].Name,
	}
	if strings.Join(sessionNames, ",") != "unread,preset-one,preset-two" {
		t.Fatalf("in-progress sessions = %v", sessionNames)
	}
	if model.InProgress.Sessions[1].Pinned != true {
		t.Fatalf("preset session is not pinned: %#v", model.InProgress.Sessions[1])
	}
}

func TestBuildNowModelDegradesForAnyNonCurrentSource(t *testing.T) {
	t.Parallel()

	sources := healthyNowSources()
	sources.Tmux.Status = nowSourceStale
	model := buildNowModel(nowModelInput{
		GeneratedAt: time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC),
		Metrics:     normalNowPosture(),
		Sources:     sources,
	})
	if model.Reliability.State != nowReliabilityDegraded {
		t.Fatalf("reliability = %q, want degraded", model.Reliability.State)
	}
	if model.Attention.Total != 0 || len(model.Attention.Visible) != 0 {
		t.Fatalf("attention = %+v, want empty", model.Attention)
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

func normalNowPosture() opsplane.MetricPosture {
	return opsplane.MetricPosture{
		State:    opsplane.MetricPostureStateNormal,
		Severity: opsplane.MetricPostureSeverityOK,
		Signals:  []opsplane.MetricPostureSignal{},
	}
}

func healthyNowSources() nowSources {
	checkedAt := "2026-07-27T10:00:00Z"
	return nowSources{
		Tmux:     nowSource{Status: nowSourceCurrent, CheckedAt: checkedAt},
		Services: nowSource{Status: nowSourceCurrent, CheckedAt: checkedAt},
		Metrics:  nowSource{Status: nowSourceCurrent, CheckedAt: checkedAt},
		Runbooks: nowSource{Status: nowSourceCurrent, CheckedAt: checkedAt},
	}
}
