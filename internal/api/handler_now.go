package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/runbook"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

type nowTmuxResult struct {
	sessions []enrichedSession
	presets  []store.SessionPreset
	source   nowSource
}

type nowServicesResult struct {
	services []opsplane.ServiceStatus
	source   nowSource
}

type nowMetricsResult struct {
	posture opsplane.MetricPosture
	source  nowSource
}

type nowRunbooksResult struct {
	runbooks       []store.OpsRunbook
	active         []store.OpsRunbookRun
	latestTerminal []store.OpsRunbookRun
	source         nowSource
}

func (h *Handler) now(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.ops == nil || h.runbooks == nil {
		writeError(w, http.StatusServiceUnavailable, "NOW_UNAVAILABLE", "Now dependencies are unavailable", nil)
		return
	}

	var (
		tmuxResult     nowTmuxResult
		servicesResult nowServicesResult
		metricsResult  nowMetricsResult
		runbooksResult nowRunbooksResult
		wg             sync.WaitGroup
	)
	wg.Add(4)

	go func() {
		defer wg.Done()
		tmuxResult = h.loadNowTmux(r.Context())
	}()
	go func() {
		defer wg.Done()
		servicesResult = h.loadNowServices(r.Context())
	}()
	go func() {
		defer wg.Done()
		metricsResult = h.loadNowMetrics(r.Context())
	}()
	go func() {
		defer wg.Done()
		runbooksResult = h.loadNowRunbooks(r.Context())
	}()
	wg.Wait()

	model := buildNowModel(nowModelInput{
		GeneratedAt:        time.Now().UTC(),
		Services:           servicesResult.services,
		Metrics:            metricsResult.posture,
		Runbooks:           runbooksResult.runbooks,
		ActiveRuns:         runbooksResult.active,
		LatestTerminalRuns: runbooksResult.latestTerminal,
		Sessions:           tmuxResult.sessions,
		SessionPresets:     tmuxResult.presets,
		Sources: nowSources{
			Tmux:     tmuxResult.source,
			Services: servicesResult.source,
			Metrics:  metricsResult.source,
			Runbooks: runbooksResult.source,
		},
	})
	writeData(w, http.StatusOK, map[string]any{"now": model})
}

func (h *Handler) loadNowTmux(ctx context.Context) nowTmuxResult {
	snapshot := h.loadEnrichedSessions(ctx)
	result := nowTmuxResult{
		sessions: snapshot.Sessions,
		source:   checkedNowSource(snapshot.Status, snapshot.Message),
		presets:  []store.SessionPreset{},
	}
	presets, err := h.repo.ListSessionPresets(ctx)
	if err == nil {
		result.presets = presets
		return result
	}
	if len(snapshot.Sessions) > 0 {
		result.source.Status = nowSourceStale
		result.source.Message = "tmux_presets_stale"
	} else {
		result.source.Status = nowSourceUnavailable
		result.source.Message = "tmux_presets_unavailable"
	}
	return result
}

func (h *Handler) loadNowServices(ctx context.Context) nowServicesResult {
	services, err := h.ops.ListServices(ctx)
	if err != nil {
		return nowServicesResult{
			services: []opsplane.ServiceStatus{},
			source:   checkedNowSource(nowSourceUnavailable, "services_unavailable"),
		}
	}
	if services == nil {
		services = []opsplane.ServiceStatus{}
	}
	return nowServicesResult{
		services: services,
		source:   checkedNowSource(nowSourceCurrent, ""),
	}
}

func (h *Handler) loadNowMetrics(ctx context.Context) nowMetricsResult {
	posture := opsplane.EvaluateMetricPosture(h.ops.Metrics(ctx))
	if posture.State == opsplane.MetricPostureStateUnavailable {
		return nowMetricsResult{
			posture: posture,
			source:  checkedNowSource(nowSourceUnavailable, "metrics_unavailable"),
		}
	}
	return nowMetricsResult{
		posture: posture,
		source:  checkedNowSource(nowSourceCurrent, ""),
	}
}

func (h *Handler) loadNowRunbooks(ctx context.Context) nowRunbooksResult {
	runbooks, err := h.runbooks.List(ctx)
	if err != nil {
		return unavailableNowRunbooks()
	}
	active, err := h.repo.ListOpsRunbookActiveRuns(ctx)
	if err != nil {
		return unavailableNowRunbooks()
	}
	latestTerminal, err := h.repo.ListOpsRunbookLatestTerminalRuns(ctx)
	if err != nil {
		return unavailableNowRunbooks()
	}
	if runbooks == nil {
		runbooks = []store.OpsRunbook{}
	}
	if active == nil {
		active = []store.OpsRunbookRun{}
	}
	if latestTerminal == nil {
		latestTerminal = []store.OpsRunbookRun{}
	}
	return nowRunbooksResult{
		runbooks:       runbooks,
		active:         active,
		latestTerminal: latestTerminal,
		source:         checkedNowSource(nowSourceCurrent, ""),
	}
}

func unavailableNowRunbooks() nowRunbooksResult {
	return nowRunbooksResult{
		runbooks:       []store.OpsRunbook{},
		active:         []store.OpsRunbookRun{},
		latestTerminal: []store.OpsRunbookRun{},
		source:         checkedNowSource(nowSourceUnavailable, "runbooks_unavailable"),
	}
}

func checkedNowSource(status, message string) nowSource {
	return nowSource{
		Status:    status,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   message,
	}
}

func (h *Handler) runNowServiceRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.ops == nil || h.runbooks == nil {
		writeError(w, http.StatusServiceUnavailable, "NOW_UNAVAILABLE", "Now dependencies are unavailable", nil)
		return
	}
	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
		return
	}

	var request runOpsRunbookRequest
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	services, err := h.ops.ListServices(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "NOW_SERVICES_UNAVAILABLE", "service state could not be verified", nil)
		return
	}
	if !isNowServiceFailed(services, serviceName) {
		writeError(w, http.StatusConflict, "NOW_SERVICE_NOT_FAILED", "service is no longer failed", nil)
		return
	}

	runbooks, err := h.runbooks.List(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "NOW_RUNBOOKS_UNAVAILABLE", "runbooks could not be loaded", nil)
		return
	}
	associated := make([]store.OpsRunbook, 0, 1)
	for _, candidate := range runbooks {
		if candidate.TargetService == serviceName {
			associated = append(associated, candidate)
		}
	}
	if len(associated) == 0 {
		writeError(w, http.StatusNotFound, "NOW_RUNBOOK_NOT_FOUND", "service has no associated runbook", nil)
		return
	}
	if len(associated) > 1 {
		writeError(w, http.StatusConflict, "NOW_RUNBOOK_CONFLICT", "service has multiple associated runbooks", nil)
		return
	}
	if !associated[0].Enabled {
		writeError(w, http.StatusConflict, "NOW_RUNBOOK_DISABLED", "associated runbook is disabled", nil)
		return
	}

	job, err := h.runbooks.StartFromNow(ctx, associated[0].ID, request.Parameters)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "NOW_RUNBOOK_NOT_FOUND", "associated runbook no longer exists", nil)
		case errors.Is(err, runbook.ErrTooManyExecutions):
			writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", err.Error(), nil)
		case errors.Is(err, runbook.ErrInvalidParameters):
			writeError(w, http.StatusBadRequest, "INVALID_PARAMETERS", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run associated runbook", nil)
		}
		return
	}

	writeData(w, http.StatusAccepted, map[string]any{
		keyJob:       job,
		keyGlobalRev: time.Now().UTC().UnixMilli(),
	})
}

func isNowServiceFailed(services []opsplane.ServiceStatus, name string) bool {
	for _, service := range services {
		if service.Name == name {
			return strings.EqualFold(strings.TrimSpace(service.ActiveState), stateFailed)
		}
	}
	return false
}
