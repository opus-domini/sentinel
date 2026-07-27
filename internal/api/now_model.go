package api

import (
	"sort"
	"strings"
	"time"

	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

const (
	nowSourceCurrent       = "current"
	nowSourceStale         = "stale"
	nowSourceUnavailable   = "unavailable"
	nowSourceNotConfigured = "not_configured"

	nowConfidenceCurrent  = "current"
	nowConfidenceDegraded = "degraded"

	nowPostureHealthy = "healthy"
	nowPostureAtRisk  = "at_risk"
	nowPostureUnknown = "unknown"

	nowAttentionRunbookApproval = "runbook_approval"
	nowAttentionServiceFailed   = "service_failed"
	nowAttentionMetricsPressure = "metrics_pressure"

	nowAttentionLimit  = 5
	nowInProgressLimit = 3
)

type nowSource struct {
	Status     string `json:"status"`
	ObservedAt string `json:"observedAt"`
	Message    string `json:"message,omitempty"`
}

type nowSources struct {
	Tmux     nowSource `json:"tmux"`
	Services nowSource `json:"services"`
	Metrics  nowSource `json:"metrics"`
	Runbooks nowSource `json:"runbooks"`
}

type nowConfidence struct {
	State   string     `json:"state"`
	Sources nowSources `json:"sources"`
}

type nowServiceSummary struct {
	Tracked  int `json:"tracked"`
	Running  int `json:"running"`
	Failed   int `json:"failed"`
	Inactive int `json:"inactive"`
	Unknown  int `json:"unknown"`
}

type nowPosture struct {
	State    string                 `json:"state"`
	Services nowServiceSummary      `json:"services"`
	Metrics  opsplane.MetricPosture `json:"metrics"`
}

type nowRunbookReference struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	Parameters    []store.RunbookParameter `json:"parameters"`
	TargetService string                   `json:"targetService,omitempty"`
	Steps         []store.OpsRunbookStep   `json:"steps"`
}

type nowServiceReference struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	TrackingMode string `json:"trackingMode"`
	Manager      string `json:"manager"`
	Scope        string `json:"scope"`
	Unit         string `json:"unit"`
}

type nowRunReference struct {
	RunbookID   string `json:"runbookId"`
	RunbookName string `json:"runbookName"`
	RunID       string `json:"runId"`
	Status      string `json:"status"`
	Source      string `json:"source,omitempty"`
	TargetKind  string `json:"targetKind,omitempty"`
	TargetName  string `json:"targetName,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

// nowAttentionItem is a discriminated union. Fields outside the selected Type
// are omitted, keeping each API item compact while preserving deterministic
// prioritization and overflow accounting.
type nowAttentionItem struct {
	Type       string                         `json:"type"`
	Service    *nowServiceReference           `json:"service,omitempty"`
	Runbook    *nowRunbookReference           `json:"runbook,omitempty"`
	Run        *nowRunReference               `json:"run,omitempty"`
	Severity   string                         `json:"severity,omitempty"`
	Signals    []opsplane.MetricPostureSignal `json:"signals,omitempty"`
	ObservedAt string                         `json:"observedAt,omitempty"`
}

type nowAttentionOverflow struct {
	Approvals int `json:"approvals"`
	Services  int `json:"services"`
	Metrics   int `json:"metrics"`
}

type nowAttention struct {
	Total    int                  `json:"total"`
	Visible  []nowAttentionItem   `json:"visible"`
	Overflow nowAttentionOverflow `json:"overflow"`
}

type nowInProgressRun struct {
	ID             string `json:"id"`
	RunbookID      string `json:"runbookId"`
	RunbookName    string `json:"runbookName"`
	Status         string `json:"status"`
	TotalSteps     int    `json:"totalSteps"`
	CompletedSteps int    `json:"completedSteps"`
	CurrentStep    string `json:"currentStep,omitempty"`
	Source         string `json:"source,omitempty"`
	TargetKind     string `json:"targetKind,omitempty"`
	TargetName     string `json:"targetName,omitempty"`
	CreatedAt      string `json:"createdAt"`
	StartedAt      string `json:"startedAt,omitempty"`
}

type nowInProgressSession struct {
	Name          string `json:"name"`
	User          string `json:"user,omitempty"`
	Pinned        bool   `json:"pinned"`
	UnreadWindows int    `json:"unreadWindows"`
	UnreadPanes   int    `json:"unreadPanes"`
	ActivityAt    string `json:"activityAt"`
}

type nowInProgress struct {
	Runs     []nowInProgressRun     `json:"runs"`
	Sessions []nowInProgressSession `json:"sessions"`
}

type nowResponse struct {
	GeneratedAt string        `json:"generatedAt"`
	Confidence  nowConfidence `json:"confidence"`
	Posture     nowPosture    `json:"posture"`
	Attention   nowAttention  `json:"attention"`
	InProgress  nowInProgress `json:"inProgress"`
}

type nowModelInput struct {
	GeneratedAt    time.Time
	Services       []opsplane.ServiceStatus
	Metrics        opsplane.MetricPosture
	Runbooks       []store.OpsRunbook
	ActiveRuns     []store.OpsRunbookRun
	Sessions       []enrichedSession
	SessionPresets []store.SessionPreset
	Sources        nowSources
}

func buildNowModel(input nowModelInput) nowResponse {
	generatedAt := input.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	metrics := normalizeNowMetricPosture(input.Metrics, generatedAt)

	runbooksByTarget := make(map[string]store.OpsRunbook, len(input.Runbooks))
	for _, runbook := range input.Runbooks {
		target := strings.TrimSpace(runbook.TargetService)
		if target != "" {
			runbooksByTarget[target] = runbook
		}
	}

	serviceSummary, failedServices := summarizeNowServices(input.Services)
	sort.SliceStable(failedServices, func(i, j int) bool {
		return canonicalServiceName(failedServices[i]) < canonicalServiceName(failedServices[j])
	})
	serviceItems := make([]nowAttentionItem, 0, len(failedServices))
	for _, service := range failedServices {
		runbook, hasRunbook := runbooksByTarget[service.Name]
		item := nowAttentionItem{
			Type:    nowAttentionServiceFailed,
			Service: nowServiceReferenceFromStatus(service),
		}
		if hasRunbook && runbook.Enabled {
			item.Runbook = nowRunbookReferenceFromStore(runbook)
		}
		serviceItems = append(serviceItems, item)
	}

	approvals := filterNowRunsByStatus(input.ActiveRuns, store.OpsRunbookStatusWaitingApproval)
	sort.SliceStable(approvals, func(i, j int) bool {
		return compareRunCreatedAt(approvals[i], approvals[j], true)
	})
	approvalItems := make([]nowAttentionItem, 0, len(approvals))
	for _, run := range approvals {
		approvalItems = append(approvalItems, nowAttentionItem{
			Type: nowAttentionRunbookApproval,
			Run:  nowRunReferenceFromStore(run),
		})
	}

	var metricsItem *nowAttentionItem
	if metrics.State == opsplane.MetricPostureStatePressure {
		item := nowAttentionItem{
			Type:       nowAttentionMetricsPressure,
			Severity:   metrics.Severity,
			Signals:    nonNilMetricSignals(metrics.Signals),
			ObservedAt: metrics.ObservedAt,
		}
		metricsItem = &item
	}

	attention := buildNowAttention(serviceItems, approvalItems, metricsItem)

	confidenceState := nowConfidenceCurrent
	if nowSourcesDegraded(input.Sources) {
		confidenceState = nowConfidenceDegraded
	}

	postureState := nowPostureHealthy
	if input.Sources.Services.Status != nowSourceCurrent ||
		input.Sources.Metrics.Status != nowSourceCurrent {
		postureState = nowPostureUnknown
	} else if serviceSummary.Failed > 0 ||
		metrics.State == opsplane.MetricPostureStatePressure {
		postureState = nowPostureAtRisk
	}

	return nowResponse{
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Confidence: nowConfidence{
			State:   confidenceState,
			Sources: input.Sources,
		},
		Posture: nowPosture{
			State:    postureState,
			Services: serviceSummary,
			Metrics:  metrics,
		},
		Attention: attention,
		InProgress: nowInProgress{
			Runs:     buildNowInProgressRuns(input.ActiveRuns),
			Sessions: buildNowInProgressSessions(input.Sessions, input.SessionPresets),
		},
	}
}

func buildNowAttention(
	services []nowAttentionItem,
	approvals []nowAttentionItem,
	metrics *nowAttentionItem,
) nowAttention {
	total := len(services) + len(approvals)
	if metrics != nil {
		total++
	}

	visible := make([]nowAttentionItem, 0, min(total, nowAttentionLimit))
	serviceIndex := 0
	approvalIndex := 0
	metricsSelected := false

	appendService := func() {
		if serviceIndex >= len(services) || len(visible) >= nowAttentionLimit {
			return
		}
		visible = append(visible, services[serviceIndex])
		serviceIndex++
	}
	appendApproval := func() {
		if approvalIndex >= len(approvals) || len(visible) >= nowAttentionLimit {
			return
		}
		visible = append(visible, approvals[approvalIndex])
		approvalIndex++
	}
	appendMetrics := func() {
		if metrics == nil || metricsSelected || len(visible) >= nowAttentionLimit {
			return
		}
		visible = append(visible, *metrics)
		metricsSelected = true
	}

	// Reserve one slot per non-empty category in operational priority order.
	appendService()
	if metrics != nil && metrics.Severity == opsplane.MetricPostureSeverityCritical {
		appendMetrics()
	}
	appendApproval()
	if metrics != nil && metrics.Severity != opsplane.MetricPostureSeverityCritical {
		appendMetrics()
	}

	// Remaining capacity favors concrete failed services, then oldest approvals.
	for serviceIndex < len(services) && len(visible) < nowAttentionLimit {
		appendService()
	}
	for approvalIndex < len(approvals) && len(visible) < nowAttentionLimit {
		appendApproval()
	}

	metricsOverflow := 0
	if metrics != nil && !metricsSelected {
		metricsOverflow = 1
	}
	return nowAttention{
		Total:   total,
		Visible: visible,
		Overflow: nowAttentionOverflow{
			Approvals: len(approvals) - approvalIndex,
			Services:  len(services) - serviceIndex,
			Metrics:   metricsOverflow,
		},
	}
}

func summarizeNowServices(services []opsplane.ServiceStatus) (nowServiceSummary, []opsplane.ServiceStatus) {
	summary := nowServiceSummary{Tracked: len(services)}
	failed := make([]opsplane.ServiceStatus, 0)
	for _, service := range services {
		switch strings.ToLower(strings.TrimSpace(service.ActiveState)) {
		case "active", "running":
			summary.Running++
		case "failed":
			summary.Failed++
			failed = append(failed, service)
		case "inactive", "dead":
			summary.Inactive++
		default:
			summary.Unknown++
		}
	}
	return summary, failed
}

func buildNowInProgressRuns(active []store.OpsRunbookRun) []nowInProgressRun {
	runs := make([]store.OpsRunbookRun, 0, len(active))
	for _, run := range active {
		if run.Status == store.OpsRunbookStatusQueued || run.Status == store.OpsRunbookStatusRunning {
			runs = append(runs, run)
		}
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return compareRunCreatedAt(runs[i], runs[j], false)
	})
	if len(runs) > nowInProgressLimit {
		runs = runs[:nowInProgressLimit]
	}
	out := make([]nowInProgressRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, nowInProgressRun{
			ID:             run.ID,
			RunbookID:      run.RunbookID,
			RunbookName:    run.RunbookName,
			Status:         run.Status,
			TotalSteps:     run.TotalSteps,
			CompletedSteps: run.CompletedSteps,
			CurrentStep:    run.CurrentStep,
			Source:         run.Source,
			TargetKind:     run.TargetKind,
			TargetName:     run.TargetName,
			CreatedAt:      run.CreatedAt,
			StartedAt:      run.StartedAt,
		})
	}
	return out
}

func buildNowInProgressSessions(
	sessions []enrichedSession,
	presets []store.SessionPreset,
) []nowInProgressSession {
	pinned := make(map[string]bool, len(presets))
	for _, preset := range presets {
		pinned[preset.Name] = true
	}

	selected := make([]enrichedSession, 0, len(sessions))
	for _, session := range sessions {
		if session.UnreadPanes <= 0 && session.UnreadWindows <= 0 {
			continue
		}
		selected = append(selected, session)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		leftAt := parseRFC3339(left.ActivityAt)
		rightAt := parseRFC3339(right.ActivityAt)
		switch {
		case !leftAt.Equal(rightAt):
			return leftAt.After(rightAt)
		case canonicalText(left.User) != canonicalText(right.User):
			return canonicalText(left.User) < canonicalText(right.User)
		default:
			return canonicalText(left.Name) < canonicalText(right.Name)
		}
	})
	if len(selected) > nowInProgressLimit {
		selected = selected[:nowInProgressLimit]
	}

	out := make([]nowInProgressSession, 0, len(selected))
	for _, session := range selected {
		out = append(out, nowInProgressSession{
			Name:          session.Name,
			User:          session.User,
			Pinned:        pinned[session.Name],
			UnreadWindows: session.UnreadWindows,
			UnreadPanes:   session.UnreadPanes,
			ActivityAt:    session.ActivityAt,
		})
	}
	return out
}

func filterNowRunsByStatus(runs []store.OpsRunbookRun, status string) []store.OpsRunbookRun {
	out := make([]store.OpsRunbookRun, 0, len(runs))
	for _, run := range runs {
		if run.Status == status {
			out = append(out, run)
		}
	}
	return out
}

func compareRunCreatedAt(left, right store.OpsRunbookRun, ascending bool) bool {
	leftAt := parseRFC3339(left.CreatedAt)
	rightAt := parseRFC3339(right.CreatedAt)
	if !leftAt.Equal(rightAt) {
		if ascending {
			return leftAt.Before(rightAt)
		}
		return leftAt.After(rightAt)
	}
	if ascending {
		return left.ID < right.ID
	}
	return left.ID > right.ID
}

func nowRunReferenceFromStore(run store.OpsRunbookRun) *nowRunReference {
	return &nowRunReference{
		RunbookID:   run.RunbookID,
		RunbookName: run.RunbookName,
		RunID:       run.ID,
		Status:      run.Status,
		Source:      run.Source,
		TargetKind:  run.TargetKind,
		TargetName:  run.TargetName,
		CreatedAt:   run.CreatedAt,
	}
}

func nowRunbookReferenceFromStore(runbook store.OpsRunbook) *nowRunbookReference {
	parameters := runbook.Parameters
	if parameters == nil {
		parameters = []store.RunbookParameter{}
	}
	steps := runbook.Steps
	if steps == nil {
		steps = []store.OpsRunbookStep{}
	}
	return &nowRunbookReference{
		ID:            runbook.ID,
		Name:          runbook.Name,
		Description:   runbook.Description,
		Parameters:    parameters,
		TargetService: runbook.TargetService,
		Steps:         steps,
	}
}

func nowServiceReferenceFromStatus(service opsplane.ServiceStatus) *nowServiceReference {
	return &nowServiceReference{
		Name:         service.Name,
		DisplayName:  service.DisplayName,
		TrackingMode: service.TrackingMode,
		Manager:      service.Manager,
		Scope:        service.Scope,
		Unit:         service.Unit,
	}
}

func normalizeNowMetricPosture(
	posture opsplane.MetricPosture,
	fallback time.Time,
) opsplane.MetricPosture {
	if posture.State == "" {
		posture = opsplane.MetricPosture{
			State:    opsplane.MetricPostureStateUnavailable,
			Severity: opsplane.MetricPostureSeverityUnknown,
		}
	}
	posture.Signals = nonNilMetricSignals(posture.Signals)
	if strings.TrimSpace(posture.ObservedAt) == "" {
		posture.ObservedAt = fallback.UTC().Format(time.RFC3339)
	}
	return posture
}

func nonNilMetricSignals(signals []opsplane.MetricPostureSignal) []opsplane.MetricPostureSignal {
	if signals == nil {
		return []opsplane.MetricPostureSignal{}
	}
	return signals
}

func nowSourcesDegraded(sources nowSources) bool {
	for _, source := range []nowSource{
		sources.Tmux,
		sources.Services,
		sources.Metrics,
		sources.Runbooks,
	} {
		if source.Status != nowSourceCurrent {
			return true
		}
	}
	return false
}

func canonicalServiceName(service opsplane.ServiceStatus) string {
	return canonicalText(service.Name)
}

func canonicalText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseRFC3339(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
