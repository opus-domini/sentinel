package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/events"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/timeline"
)

func (h *Handler) opsOverview(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to load ops overview", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"overview": overview,
	})
}

func (h *Handler) opsServices(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	services, err := h.ops.ListServices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to load ops services", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"services": services,
	})
}

func (h *Handler) opsServiceAction(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if !slices.Contains([]string{
		opsplane.ActionStart,
		opsplane.ActionStop,
		opsplane.ActionRestart,
	}, req.Action) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "action must be start, stop, or restart", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	serviceStatus, err := h.ops.Act(ctx, serviceName, req.Action)
	if err != nil {
		switch {
		case errors.Is(err, opsplane.ErrServiceNotFound):
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "service not found", nil)
		case errors.Is(err, opsplane.ErrInvalidAction):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid action", nil)
		default:
			slog.Warn("ops service action failed", "service", serviceName, "action", req.Action, "err", err)
			writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "service action failed", nil)
		}
		return
	}

	services, err := h.ops.ListServices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops services", nil)
		return
	}
	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops overview", nil)
		return
	}

	now := time.Now().UTC()
	timelineEvent, timelineRecorded, firedAlerts, err := h.orch.RecordServiceAction(ctx, serviceStatus, req.Action, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist ops action", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		"globalRev": globalRev,
		"service":   serviceStatus.Name,
		"action":    req.Action,
		"services":  services,
	})
	h.emit(events.TypeOpsOverview, map[string]any{
		"globalRev": globalRev,
		"overview":  overview,
	})
	if timelineRecorded {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     timelineEvent,
		})
	}
	if len(firedAlerts) > 0 {
		h.emit(events.TypeOpsAlerts, map[string]any{
			"globalRev": globalRev,
			"alerts":    firedAlerts,
		})
	}

	response := map[string]any{
		"service":   serviceStatus,
		"services":  services,
		"overview":  overview,
		"alerts":    firedAlerts,
		"globalRev": globalRev,
	}
	if timelineRecorded {
		response["timelineEvent"] = timelineEvent
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) opsServiceStatus(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := h.ops.Inspect(ctx, serviceName)
	if err != nil {
		if errors.Is(err, opsplane.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "service not found", nil)
			return
		}
		slog.Warn("ops service inspect failed", "service", serviceName, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "failed to inspect service", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"status": status,
	})
}

func (h *Handler) opsAlerts(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	limit, err := parseTimelineLimitParam(strings.TrimSpace(r.URL.Query().Get("limit")), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	alertsList, err := h.store.ListAlerts(ctx, limit, status)
	if err != nil {
		if errors.Is(err, alerts.ErrInvalidFilter) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load alerts", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"alerts": alertsList,
	})
}

func (h *Handler) ackOpsAlert(w http.ResponseWriter, r *http.Request) {
	alertRaw := strings.TrimSpace(r.PathValue("alert"))
	alertID, err := strconv.ParseInt(alertRaw, 10, 64)
	if err != nil || alertID <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "alert must be a positive integer", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	now := time.Now().UTC()
	alert, timelineEvent, timelineRecorded, err := h.orch.AckAlert(ctx, alertID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_ALERT_NOT_FOUND", "alert not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to ack alert", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsAlerts, map[string]any{
		"globalRev": globalRev,
		"alert":     alert,
		"action":    "ack",
	})
	if timelineRecorded {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     timelineEvent,
		})
	}

	writeData(w, http.StatusOK, map[string]any{
		"alert":         alert,
		"timelineEvent": timelineEvent,
		"globalRev":     globalRev,
	})
}

func (h *Handler) opsTimeline(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	limit, err := parseTimelineLimitParam(strings.TrimSpace(r.URL.Query().Get("limit")), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	query := timeline.Query{
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Severity: strings.TrimSpace(r.URL.Query().Get("severity")),
		Source:   strings.TrimSpace(r.URL.Query().Get("source")),
		Limit:    limit,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	result, err := h.store.SearchTimelineEvents(ctx, query)
	if err != nil {
		if errors.Is(err, timeline.ErrInvalidFilter) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to query ops timeline", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"events":  result.Events,
		"hasMore": result.HasMore,
	})
}

func (h *Handler) registerOpsService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Manager     string `json:"manager"`
		Unit        string `json:"unit"`
		Scope       string `json:"scope"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required", nil)
		return
	}
	if strings.TrimSpace(req.Unit) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	now := time.Now().UTC()
	te, err := h.orch.RegisterService(ctx, store.CustomServiceWrite{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Manager:     req.Manager,
		Unit:        req.Unit,
		Scope:       req.Scope,
	}, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "OPS_SERVICE_EXISTS", "service already registered", nil)
		} else {
			slog.Warn("register ops service failed", "name", req.Name, "err", err)
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to register service", nil)
		}
		return
	}

	// Re-fetch the full services list so the new service is probed.
	var services []opsplane.ServiceStatus
	if h.ops != nil {
		services, _ = h.ops.ListServices(ctx)
	}
	if services == nil {
		services = []opsplane.ServiceStatus{}
	}

	globalRev := now.UnixMilli()
	if te.ID > 0 {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     te,
		})
	}
	h.emit(events.TypeOpsServices, map[string]any{
		"globalRev": globalRev,
		"action":    "registered",
		"service":   req.Name,
	})

	writeData(w, http.StatusCreated, map[string]any{
		"services":  services,
		"globalRev": globalRev,
	})
}

func (h *Handler) unregisterOpsService(w http.ResponseWriter, r *http.Request) {
	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service name is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	now := time.Now().UTC()
	te, err := h.orch.UnregisterService(ctx, serviceName, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "custom service not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to unregister service", nil)
		return
	}

	globalRev := now.UnixMilli()
	if te.ID > 0 {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     te,
		})
	}
	h.emit(events.TypeOpsServices, map[string]any{
		"globalRev": globalRev,
		"action":    "unregistered",
		"service":   serviceName,
	})

	writeData(w, http.StatusOK, map[string]any{
		"removed":   serviceName,
		"globalRev": globalRev,
	})
}

func (h *Handler) opsServiceLogs(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service name is required", nil)
		return
	}

	lines := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			lines = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	output, err := h.ops.Logs(ctx, serviceName, lines)
	if err != nil {
		if errors.Is(err, opsplane.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "service not found", nil)
			return
		}
		slog.Warn("ops service logs failed", "service", serviceName, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_LOGS_FAILED", "failed to fetch service logs", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"service": serviceName,
		"lines":   lines,
		"output":  output,
	})
}

func (h *Handler) discoverOpsServices(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	available, err := h.ops.DiscoverServices(ctx)
	if err != nil {
		slog.Warn("ops discover services failed", "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_DISCOVER_FAILED", "failed to discover services", nil)
		return
	}
	if available == nil {
		available = []opsplane.AvailableService{}
	}
	writeData(w, http.StatusOK, map[string]any{
		"services": available,
	})
}

func (h *Handler) browseOpsServices(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	services, err := h.ops.BrowseServices(ctx)
	if err != nil {
		slog.Warn("ops browse services failed", "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_BROWSE_FAILED", "failed to browse services", nil)
		return
	}
	if services == nil {
		services = []opsplane.BrowsedService{}
	}
	writeData(w, http.StatusOK, map[string]any{
		"services": services,
	})
}

func (h *Handler) opsUnitAction(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	var req struct {
		Unit    string `json:"unit"`
		Scope   string `json:"scope"`
		Manager string `json:"manager"`
		Action  string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Unit = strings.TrimSpace(req.Unit)
	req.Scope = strings.TrimSpace(req.Scope)
	req.Manager = strings.TrimSpace(req.Manager)
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))

	if req.Unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}
	if !slices.Contains([]string{opsplane.ActionStart, opsplane.ActionStop, opsplane.ActionRestart}, req.Action) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "action must be start, stop, or restart", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	if err := h.ops.ActByUnit(ctx, req.Unit, req.Scope, req.Manager, req.Action); err != nil {
		if errors.Is(err, opsplane.ErrInvalidAction) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid action", nil)
		} else {
			slog.Warn("ops unit action failed", "unit", req.Unit, "action", req.Action, "err", err)
			writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "unit action failed", nil)
		}
		return
	}

	now := time.Now().UTC()
	serviceStatus := opsplane.ServiceStatus{
		Name:        req.Unit,
		DisplayName: req.Unit,
		Unit:        req.Unit,
		Scope:       req.Scope,
		Manager:     req.Manager,
	}

	timelineEvent, timelineRecorded, firedAlerts, err := h.orch.RecordServiceAction(ctx, serviceStatus, req.Action, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist ops action", nil)
		return
	}

	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops overview", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		"globalRev": globalRev,
		"service":   req.Unit,
		"action":    req.Action,
	})
	h.emit(events.TypeOpsOverview, map[string]any{
		"globalRev": globalRev,
		"overview":  overview,
	})
	if timelineRecorded {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     timelineEvent,
		})
	}

	response := map[string]any{
		"overview":  overview,
		"alerts":    firedAlerts,
		"globalRev": globalRev,
	}
	if timelineRecorded {
		response["timelineEvent"] = timelineEvent
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) opsUnitStatus(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	manager := strings.TrimSpace(r.URL.Query().Get("manager"))

	if unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := h.ops.InspectByUnit(ctx, unit, scope, manager)
	if err != nil {
		slog.Warn("ops unit inspect failed", "unit", unit, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "failed to inspect unit", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"status": status,
	})
}

func (h *Handler) opsUnitLogs(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	manager := strings.TrimSpace(r.URL.Query().Get("manager"))

	if unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}

	lines := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			lines = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	output, err := h.ops.LogsByUnit(ctx, unit, scope, manager, lines)
	if err != nil {
		slog.Warn("ops unit logs failed", "unit", unit, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_LOGS_FAILED", "failed to fetch unit logs", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"unit":   unit,
		"lines":  lines,
		"output": output,
	})
}

func (h *Handler) opsMetrics(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	metrics := h.ops.Metrics(ctx)
	writeData(w, http.StatusOK, map[string]any{
		"metrics": metrics,
	})
}
