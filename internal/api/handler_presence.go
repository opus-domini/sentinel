package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) setTmuxPresence(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	var req struct {
		TerminalID  string `json:"terminalId"`
		SessionName string `json:"session"`
		WindowIndex int    `json:"windowIndex"`
		PaneID      string `json:"paneId"`
		Visible     bool   `json:"visible"`
		Focused     bool   `json:"focused"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.TerminalID = strings.TrimSpace(req.TerminalID)
	req.SessionName = strings.TrimSpace(req.SessionName)
	req.PaneID = strings.TrimSpace(req.PaneID)

	if req.TerminalID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "terminalId is required", nil)
		return
	}
	if req.SessionName != "" && !validate.SessionName(req.SessionName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	if req.WindowIndex < -1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "windowIndex must be >= -1", nil)
		return
	}
	if req.PaneID != "" && !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	now := time.Now().UTC()
	expiresAt := now.Add(events.PresenceExpiry)
	if err := h.repo.UpsertWatchtowerPresence(ctx, store.WatchtowerPresenceWrite{
		TerminalID:  req.TerminalID,
		SessionName: req.SessionName,
		WindowIndex: req.WindowIndex,
		PaneID:      req.PaneID,
		Visible:     req.Visible,
		Focused:     req.Focused,
		UpdatedAt:   now,
		ExpiresAt:   expiresAt,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to set presence", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
}

func (h *Handler) activityDelta(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	since, limit, err := parseActivityDeltaParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	changes, err := h.repo.ListWatchtowerJournalSince(ctx, since, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to read activity delta", nil)
		return
	}
	overflow := false
	if len(changes) > limit {
		overflow = true
		changes = changes[:limit]
	}

	globalRev := readWatchtowerGlobalRev(ctx, h.repo)
	sessionNames := extractChangedSessionNames(changes)
	sessionPatches, inspectorPatches := h.collectSessionsPatches(ctx, sessionNames)
	response := map[string]any{
		"since":      since,
		"limit":      limit,
		keyGlobalRev: globalRev,
		"overflow":   overflow,
		"changes":    changes,
	}
	if len(sessionPatches) > 0 {
		response["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		response["inspectorPatches"] = inspectorPatches
	}
	writeData(w, http.StatusOK, response)
}

func parseActivityDeltaParams(r *http.Request) (int64, int, error) {
	since := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("since must be >= 0")
		}
		since = parsed
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return 0, 0, errors.New("limit must be > 0")
		}
		if parsed > 1000 {
			parsed = 1000
		}
		limit = parsed
	}
	return since, limit, nil
}

func extractChangedSessionNames(changes []store.WatchtowerJournal) []string {
	sessionSet := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		session := strings.TrimSpace(change.Session)
		if session == "" {
			continue
		}
		sessionSet[session] = struct{}{}
	}
	names := make([]string, 0, len(sessionSet))
	for session := range sessionSet {
		names = append(names, session)
	}
	return names
}

func (h *Handler) collectSessionsPatches(ctx context.Context, sessions []string) ([]map[string]any, []map[string]any) {
	sessionPatches := make([]map[string]any, 0, len(sessions))
	inspectorPatches := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		if patch, err := h.repo.GetWatchtowerSessionActivityPatch(ctx, session); err == nil {
			sessionPatches = append(sessionPatches, patch)
		}
		if patch, err := h.repo.GetWatchtowerInspectorPatch(ctx, session); err == nil {
			inspectorPatches = append(inspectorPatches, patch)
		}
	}
	return sessionPatches, inspectorPatches
}

func (h *Handler) activityStats(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	keys := []string{
		"global_rev",
		"collect_total",
		"collect_errors_total",
		"last_collect_at",
		"last_collect_duration_ms",
		"last_collect_sessions",
		"last_collect_changed_sessions",
		"last_collect_error",
	}

	runtime := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := h.repo.GetWatchtowerRuntimeValue(ctx, key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to read activity stats", nil)
			return
		}
		runtime[key] = strings.TrimSpace(value)
	}

	parseInt := func(key string) int64 {
		raw := strings.TrimSpace(runtime[key])
		if raw == "" {
			return 0
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	}

	writeData(w, http.StatusOK, map[string]any{
		keyGlobalRev:            parseInt("global_rev"),
		"collectTotal":          parseInt("collect_total"),
		"collectErrorsTotal":    parseInt("collect_errors_total"),
		"lastCollectAt":         runtime["last_collect_at"],
		"lastCollectDurationMs": parseInt("last_collect_duration_ms"),
		"lastCollectSessions":   parseInt("last_collect_sessions"),
		"lastCollectChanged":    parseInt("last_collect_changed_sessions"),
		"lastCollectError":      runtime["last_collect_error"],
		"runtime":               runtime,
	})
}
