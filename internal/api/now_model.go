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

	nowReliabilityNormal    = "normal"
	nowReliabilityAttention = "attention"
	nowReliabilityDegraded  = "degraded"

	nowAttentionRunbookApproval = "runbook_approval"
	nowAttentionServiceFailed   = "service_failed"
	nowAttentionRunbookFailed   = "runbook_failed"
	nowAttentionMetricsPressure = "metrics_pressure"

	nowAttentionLimit  = 5
	nowInProgressLimit = 3

	nowCategoryApprovals = "approvals"
)

type nowSource struct {
	Status    string `json:"status"`
	CheckedAt string `json:"checkedAt"`
	Message   string `json:"message,omitempty"`
}

type nowSources struct {
	Tmux     nowSource `json:"tmux"`
	Services nowSource `json:"services"`
	Metrics  nowSource `json:"metrics"`
	Runbooks nowSource `json:"runbooks"`
}

type nowServiceSummary struct {
	Tracked  int `json:"tracked"`
	Running  int `json:"running"`
	Failed   int `json:"failed"`
	Inactive int `json:"inactive"`
	Unknown  int `json:"unknown"`
}

type nowReliability struct {
	State    string                 `json:"state"`
	Services nowServiceSummary      `json:"services"`
	Metrics  opsplane.MetricPosture `json:"metrics"`
}

type nowRunbookReference struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Parameters  []store.RunbookParameter `json:"parameters"`
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
// are omitted, keeping each API item compact while preserving one typed Go
// representation for deterministic prioritization and overflow accounting.
type nowAttentionItem struct {
	Type     string                         `json:"type"`
	Service  *nowServiceReference           `json:"service,omitempty"`
	Runbook  *nowRunbookReference           `json:"runbook,omitempty"`
	Failure  *nowRunReference               `json:"failure,omitempty"`
	Run      *nowRunReference               `json:"run,omitempty"`
	Severity string                         `json:"severity,omitempty"`
	Signals  []opsplane.MetricPostureSignal `json:"signals,omitempty"`
}

type nowAttentionOverflow struct {
	Approvals int `json:"approvals"`
	Services  int `json:"services"`
	Runbooks  int `json:"runbooks"`
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
	GeneratedAt string         `json:"generatedAt"`
	Reliability nowReliability `json:"reliability"`
	Attention   nowAttention   `json:"attention"`
	InProgress  nowInProgress  `json:"inProgress"`
	Sources     nowSources     `json:"sources"`
}

type nowModelInput struct {
	GeneratedAt        time.Time
	Services           []opsplane.ServiceStatus
	Metrics            opsplane.MetricPosture
	Runbooks           []store.OpsRunbook
	ActiveRuns         []store.OpsRunbookRun
	LatestTerminalRuns []store.OpsRunbookRun
	Sessions           []enrichedSession
	SessionPresets     []store.SessionPreset
	Sources            nowSources
}

type nowAttentionCandidate struct {
	category string
	item     nowAttentionItem
}

func buildNowModel(input nowModelInput) nowResponse {
	runbooksByTarget := make(map[string]store.OpsRunbook, len(input.Runbooks))
	for _, runbook := range input.Runbooks {
		target := strings.TrimSpace(runbook.TargetService)
		if target != "" {
			runbooksByTarget[target] = runbook
		}
	}

	serviceSummary, failedServices := summarizeNowServices(input.Services)
	failedByName := make(map[string]int, len(failedServices))
	candidates := make([]nowAttentionCandidate, 0, len(input.ActiveRuns)+len(failedServices)+len(input.LatestTerminalRuns)+1)

	approvals := filterNowRunsByStatus(input.ActiveRuns, store.OpsRunbookStatusWaitingApproval)
	sort.SliceStable(approvals, func(i, j int) bool {
		return compareRunCreatedAt(approvals[i], approvals[j], true)
	})
	for _, run := range approvals {
		candidates = append(candidates, nowAttentionCandidate{
			category: nowCategoryApprovals,
			item: nowAttentionItem{
				Type: nowAttentionRunbookApproval,
				Run:  nowRunReferenceFromStore(run),
			},
		})
	}

	sort.SliceStable(failedServices, func(i, j int) bool {
		return canonicalServiceName(failedServices[i]) < canonicalServiceName(failedServices[j])
	})
	for _, service := range failedServices {
		runbook, hasRunbook := runbooksByTarget[service.Name]
		item := nowAttentionItem{
			Type:    nowAttentionServiceFailed,
			Service: nowServiceReferenceFromStatus(service),
		}
		if hasRunbook && runbook.Enabled {
			item.Runbook = nowRunbookReferenceFromStore(runbook)
		}
		failedByName[service.Name] = len(candidates)
		candidates = append(candidates, nowAttentionCandidate{
			category: keyServices,
			item:     item,
		})
	}

	failedRuns := filterNowRunsByStatus(input.LatestTerminalRuns, store.OpsRunbookStatusFailed)
	sort.SliceStable(failedRuns, func(i, j int) bool {
		return compareRunCreatedAt(failedRuns[i], failedRuns[j], false)
	})
	for _, run := range failedRuns {
		if run.TargetKind == store.OpsRunbookRunTargetService {
			if candidateIndex, ok := failedByName[run.TargetName]; ok {
				candidates[candidateIndex].item.Failure = nowRunReferenceFromStore(run)
				continue
			}
		}
		candidates = append(candidates, nowAttentionCandidate{
			category: keyRunbooks,
			item: nowAttentionItem{
				Type: nowAttentionRunbookFailed,
				Run:  nowRunReferenceFromStore(run),
			},
		})
	}

	if input.Metrics.State == opsplane.MetricPostureStatePressure {
		candidates = append(candidates, nowAttentionCandidate{
			category: keyMetrics,
			item: nowAttentionItem{
				Type:     nowAttentionMetricsPressure,
				Severity: input.Metrics.Severity,
				Signals:  nonNilMetricSignals(input.Metrics.Signals),
			},
		})
	}

	visibleCount := min(len(candidates), nowAttentionLimit)
	visible := make([]nowAttentionItem, 0, visibleCount)
	for _, candidate := range candidates[:visibleCount] {
		visible = append(visible, candidate.item)
	}
	overflow := nowAttentionOverflow{}
	for _, candidate := range candidates[visibleCount:] {
		switch candidate.category {
		case nowCategoryApprovals:
			overflow.Approvals++
		case keyServices:
			overflow.Services++
		case keyRunbooks:
			overflow.Runbooks++
		case keyMetrics:
			overflow.Metrics++
		}
	}

	reliabilityState := nowReliabilityNormal
	if nowSourcesDegraded(input.Sources) {
		reliabilityState = nowReliabilityDegraded
	} else if serviceSummary.Failed > 0 || input.Metrics.State == opsplane.MetricPostureStatePressure {
		reliabilityState = nowReliabilityAttention
	}

	generatedAt := input.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	return nowResponse{
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Reliability: nowReliability{
			State:    reliabilityState,
			Services: serviceSummary,
			Metrics:  normalizeNowMetricPosture(input.Metrics),
		},
		Attention: nowAttention{
			Total:    len(candidates),
			Visible:  visible,
			Overflow: overflow,
		},
		InProgress: nowInProgress{
			Runs:     buildNowInProgressRuns(input.ActiveRuns),
			Sessions: buildNowInProgressSessions(input.Sessions, input.SessionPresets),
		},
		Sources: input.Sources,
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

func buildNowInProgressSessions(sessions []enrichedSession, presets []store.SessionPreset) []nowInProgressSession {
	presetOrder := make(map[string]int, len(presets))
	for _, preset := range presets {
		presetOrder[preset.Name] = preset.SortOrder
	}

	type candidate struct {
		session enrichedSession
		pinned  bool
		order   int
	}
	selected := make([]candidate, 0, len(sessions))
	for _, session := range sessions {
		order, pinned := presetOrder[session.Name]
		if !pinned && session.UnreadPanes <= 0 && session.UnreadWindows <= 0 {
			continue
		}
		selected = append(selected, candidate{session: session, pinned: pinned, order: order})
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		switch {
		case left.session.UnreadPanes != right.session.UnreadPanes:
			return left.session.UnreadPanes > right.session.UnreadPanes
		case left.session.UnreadWindows != right.session.UnreadWindows:
			return left.session.UnreadWindows > right.session.UnreadWindows
		case left.pinned && right.pinned && left.order != right.order:
			return left.order < right.order
		case left.pinned != right.pinned:
			return left.pinned
		case left.session.ActivityAt != right.session.ActivityAt:
			return parseRFC3339(left.session.ActivityAt).After(parseRFC3339(right.session.ActivityAt))
		default:
			return strings.ToLower(left.session.Name) < strings.ToLower(right.session.Name)
		}
	})
	if len(selected) > nowInProgressLimit {
		selected = selected[:nowInProgressLimit]
	}

	out := make([]nowInProgressSession, 0, len(selected))
	for _, candidate := range selected {
		out = append(out, nowInProgressSession{
			Name:          candidate.session.Name,
			User:          candidate.session.User,
			Pinned:        candidate.pinned,
			UnreadWindows: candidate.session.UnreadWindows,
			UnreadPanes:   candidate.session.UnreadPanes,
			ActivityAt:    candidate.session.ActivityAt,
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
	return &nowRunbookReference{
		ID:          runbook.ID,
		Name:        runbook.Name,
		Description: runbook.Description,
		Parameters:  parameters,
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

func normalizeNowMetricPosture(posture opsplane.MetricPosture) opsplane.MetricPosture {
	if posture.State == "" {
		posture = opsplane.MetricPosture{
			State:    opsplane.MetricPostureStateUnavailable,
			Severity: opsplane.MetricPostureSeverityUnknown,
		}
	}
	posture.Signals = nonNilMetricSignals(posture.Signals)
	return posture
}

func nonNilMetricSignals(signals []opsplane.MetricPostureSignal) []opsplane.MetricPostureSignal {
	if signals == nil {
		return []opsplane.MetricPostureSignal{}
	}
	return signals
}

func nowSourcesDegraded(sources nowSources) bool {
	for _, source := range []nowSource{sources.Tmux, sources.Services, sources.Metrics, sources.Runbooks} {
		if source.Status != nowSourceCurrent {
			return true
		}
	}
	return false
}

func canonicalServiceName(service opsplane.ServiceStatus) string {
	return strings.ToLower(strings.TrimSpace(service.Name))
}

func parseRFC3339(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
