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

	"github.com/opus-domini/sentinel/internal/events"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

var (
	validManagers = []string{"systemd", "launchd"}
	validScopes   = []string{"user", "system", ""}
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
		keyOverview: overview,
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
		keyServices: services,
	})
}

func (h *Handler) opsServiceAction(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue(keyService))
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
		opsplane.ActionEnable,
		opsplane.ActionDisable,
	}, req.Action) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "action must be start, stop, restart, enable, or disable", nil)
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
			slog.Warn("ops service action failed", keyService, serviceName, keyAction, req.Action, "err", err)
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
	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		keyGlobalRev: globalRev,
		keyService:   serviceStatus.Name,
		keyAction:    req.Action,
		keyServices:  services,
	})
	h.emit(events.TypeOpsOverview, map[string]any{
		keyGlobalRev: globalRev,
		keyOverview:  overview,
	})

	response := map[string]any{
		keyService:   serviceStatus,
		keyServices:  services,
		keyOverview:  overview,
		keyGlobalRev: globalRev,
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) opsServiceStatus(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue(keyService))
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
		slog.Warn("ops service inspect failed", keyService, serviceName, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "failed to inspect service", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		keyStatus: status,
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
	if !validOpsUnitLabel(req.Unit) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is invalid", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if _, err := h.repo.InsertCustomService(ctx, store.CustomServiceWrite{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Manager:     req.Manager,
		Unit:        req.Unit,
		Scope:       req.Scope,
	}); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "OPS_SERVICE_EXISTS", "service already registered", nil)
		} else {
			slog.Warn("register ops service failed", keyName, req.Name, "err", err)
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

	globalRev := time.Now().UTC().UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		keyGlobalRev: globalRev,
		keyAction:    "registered",
		keyService:   req.Name,
	})

	writeData(w, http.StatusCreated, map[string]any{
		keyServices:  services,
		keyGlobalRev: globalRev,
	})
}

func validOpsUnitLabel(unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.HasPrefix(unit, "-") || strings.ContainsAny(unit, "\x00\n\r\t") {
		return false
	}
	return true
}

func (h *Handler) unregisterOpsService(w http.ResponseWriter, r *http.Request) {
	serviceName := strings.TrimSpace(r.PathValue(keyService))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service name is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.DeleteCustomService(ctx, serviceName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "custom service not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to unregister service", nil)
		return
	}

	globalRev := time.Now().UTC().UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		keyGlobalRev: globalRev,
		keyAction:    "unregistered",
		keyService:   serviceName,
	})

	writeData(w, http.StatusOK, map[string]any{
		keyRemoved:   serviceName,
		keyGlobalRev: globalRev,
	})
}

func (h *Handler) opsServiceLogs(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	serviceName := strings.TrimSpace(r.PathValue(keyService))
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
		slog.Warn("ops service logs failed", keyService, serviceName, "err", err)
		writeError(w, http.StatusInternalServerError, "OPS_LOGS_FAILED", "failed to fetch service logs", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		keyService: serviceName,
		"lines":    lines,
		"output":   output,
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
		keyServices: available,
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
		keyServices: services,
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
	if !slices.Contains([]string{opsplane.ActionStart, opsplane.ActionStop, opsplane.ActionRestart, opsplane.ActionEnable, opsplane.ActionDisable}, req.Action) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "action must be start, stop, restart, enable, or disable", nil)
		return
	}
	if !slices.Contains(validManagers, req.Manager) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "manager must be systemd or launchd", nil)
		return
	}
	if !slices.Contains(validScopes, req.Scope) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scope must be user or system", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	if err := h.ops.ActByUnit(ctx, req.Unit, req.Scope, req.Manager, req.Action); err != nil {
		if errors.Is(err, opsplane.ErrInvalidAction) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid action", nil)
		} else {
			slog.Warn("ops unit action failed", "unit", req.Unit, keyAction, req.Action, "err", err)
			writeError(w, http.StatusInternalServerError, "OPS_ACTION_FAILED", "unit action failed", nil)
		}
		return
	}

	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops overview", nil)
		return
	}

	globalRev := time.Now().UTC().UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		keyGlobalRev: globalRev,
		keyService:   req.Unit,
		keyAction:    req.Action,
	})
	h.emit(events.TypeOpsOverview, map[string]any{
		keyGlobalRev: globalRev,
		keyOverview:  overview,
	})
	response := map[string]any{
		keyOverview:  overview,
		keyGlobalRev: globalRev,
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) opsUnitStatus(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	scope := strings.TrimSpace(r.URL.Query().Get(keyScope))
	manager := strings.TrimSpace(r.URL.Query().Get("manager"))

	if unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}
	if !slices.Contains(validManagers, manager) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "manager must be systemd or launchd", nil)
		return
	}
	if !slices.Contains(validScopes, scope) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scope must be user or system", nil)
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
		keyStatus: status,
	})
}

func (h *Handler) opsUnitLogs(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	scope := strings.TrimSpace(r.URL.Query().Get(keyScope))
	manager := strings.TrimSpace(r.URL.Query().Get("manager"))

	if unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unit is required", nil)
		return
	}
	if !slices.Contains(validManagers, manager) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "manager must be systemd or launchd", nil)
		return
	}
	if !slices.Contains(validScopes, scope) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scope must be user or system", nil)
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
