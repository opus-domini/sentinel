package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
)

func (h *Handler) listGuardrailRules(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeData(w, http.StatusOK, map[string]any{"rules": []store.GuardrailRule{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rules, err := h.guardrails.ListRules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail rules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) updateGuardrailRule(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("rule"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "rule is required", nil)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Pattern  string `json:"pattern"`
		Mode     string `json:"mode"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Enabled  *bool  `json:"enabled"`
		Priority int    `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Pattern) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "pattern is required", nil)
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "enabled is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.guardrails.UpsertRule(ctx, store.GuardrailRuleWrite{
		ID:       ruleID,
		Name:     req.Name,
		Scope:    req.Scope,
		Pattern:  req.Pattern,
		Mode:     req.Mode,
		Severity: req.Severity,
		Message:  req.Message,
		Enabled:  *req.Enabled,
		Priority: req.Priority,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update guardrail rule", nil)
		return
	}
	rules, err := h.guardrails.ListRules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail rules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) createGuardrailRule(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}

	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Pattern  string `json:"pattern"`
		Mode     string `json:"mode"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Enabled  *bool  `json:"enabled"`
		Priority int    `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Pattern) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "pattern is required", nil)
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "enabled is required", nil)
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate rule id", nil)
			return
		}
		id = hex.EncodeToString(b)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.guardrails.UpsertRule(ctx, store.GuardrailRuleWrite{
		ID:       id,
		Name:     req.Name,
		Scope:    req.Scope,
		Pattern:  req.Pattern,
		Mode:     req.Mode,
		Severity: req.Severity,
		Message:  req.Message,
		Enabled:  *req.Enabled,
		Priority: req.Priority,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create guardrail rule", nil)
		return
	}
	rules, err := h.guardrails.ListRules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail rules", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"rules": rules})
}

func (h *Handler) deleteGuardrailRule(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("rule"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "rule is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.guardrails.DeleteRule(ctx, ruleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "guardrail rule not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete guardrail rule", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"removed": ruleID})
}

func (h *Handler) listGuardrailAudit(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeData(w, http.StatusOK, map[string]any{"audit": []store.GuardrailAudit{}})
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "limit must be > 0", nil)
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = parsed
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	auditRows, err := h.guardrails.ListAudit(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail audit", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"audit": auditRows})
}

func (h *Handler) evaluateGuardrail(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}

	var req struct {
		Action      string `json:"action"`
		SessionName string `json:"sessionName"`
		WindowIndex int    `json:"windowIndex"`
		PaneID      string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.PaneID != "" && !strings.HasPrefix(strings.TrimSpace(req.PaneID), "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	input := guardrails.Input{
		Action:      req.Action,
		SessionName: req.SessionName,
		WindowIndex: req.WindowIndex,
		PaneID:      req.PaneID,
	}
	decision, err := h.guardrails.Evaluate(ctx, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to evaluate guardrail policy", nil)
		return
	}
	if err := h.guardrails.RecordAudit(ctx, input, decision, false, "manual evaluate"); err != nil {
		slog.Warn("guardrail evaluate audit write failed", "err", err)
	}
	writeData(w, http.StatusOK, map[string]any{"decision": decision})
}

func (h *Handler) enforceGuardrail(
	w http.ResponseWriter,
	r *http.Request,
	input guardrails.Input,
) bool {
	if h == nil || h.guardrails == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	decision, err := h.guardrails.Evaluate(ctx, input)
	if err != nil {
		slog.Warn("guardrail evaluate failed, blocking request", "action", input.Action, "err", err)
		writeError(w, http.StatusServiceUnavailable, "GUARDRAIL_UNAVAILABLE", "guardrail policy could not be evaluated; action blocked for safety", nil)
		return false
	}

	confirmed := hasGuardrailConfirm(r)
	auditOverride := false
	auditReason := ""
	switch decision.Mode {
	case store.GuardrailModeBlock:
		if h.events != nil {
			h.events.Publish(events.NewEvent(events.TypeTmuxGuardrail, map[string]any{
				"action":   strings.TrimSpace(input.Action),
				"session":  strings.TrimSpace(input.SessionName),
				"paneId":   strings.TrimSpace(input.PaneID),
				"decision": decision,
			}))
		}
		if err := h.guardrails.RecordAudit(ctx, input, decision, false, "blocked"); err != nil {
			slog.Warn("guardrail audit write failed", "err", err)
		}
		writeError(w, http.StatusConflict, "GUARDRAIL_BLOCKED", decision.Message, map[string]any{
			"decision": decision,
		})
		return false
	case store.GuardrailModeConfirm:
		if !confirmed {
			if h.events != nil {
				h.events.Publish(events.NewEvent(events.TypeTmuxGuardrail, map[string]any{
					"action":   strings.TrimSpace(input.Action),
					"session":  strings.TrimSpace(input.SessionName),
					"paneId":   strings.TrimSpace(input.PaneID),
					"decision": decision,
				}))
			}
			if err := h.guardrails.RecordAudit(ctx, input, decision, false, "confirm-required"); err != nil {
				slog.Warn("guardrail audit write failed", "err", err)
			}
			writeError(w, http.StatusPreconditionRequired, "GUARDRAIL_CONFIRM_REQUIRED", decision.Message, map[string]any{
				"decision": decision,
			})
			return false
		}
		auditOverride = true
		auditReason = "confirmed"
	default:
		if decision.Mode == store.GuardrailModeWarn {
			auditReason = activity.SeverityWarn
		}
	}
	if len(decision.MatchedRules) > 0 {
		if err := h.guardrails.RecordAudit(ctx, input, decision, auditOverride, auditReason); err != nil {
			slog.Warn("guardrail audit write failed", "err", err)
		}
	}
	return true
}

func hasGuardrailConfirm(r *http.Request) bool {
	if r == nil {
		return false
	}
	candidates := []string{
		r.Header.Get("X-Sentinel-Guardrail-Confirm"),
		r.URL.Query().Get("confirm"),
	}
	for _, raw := range candidates {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "confirm", "confirmed":
			return true
		}
	}
	return false
}
