package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

func (h *Handler) listMarkerPatterns(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeData(w, http.StatusOK, map[string]any{keyPatterns: []store.MarkerPattern{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	patterns, err := h.repo.ListMarkerPatterns(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list marker patterns", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyPatterns: patterns})
}

func (h *Handler) upsertMarkerPattern(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	patternID := strings.TrimSpace(r.PathValue("pattern"))
	if patternID == "" {
		// Generate an ID for new patterns when none is specified.
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate pattern id", nil)
			return
		}
		patternID = hex.EncodeToString(b)
	}

	var req struct {
		Pattern  string `json:"pattern"`
		Severity string `json:"severity"`
		Label    string `json:"label"`
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

	if err := h.repo.UpsertMarkerPattern(ctx, store.MarkerPatternWrite{
		ID:       patternID,
		Pattern:  req.Pattern,
		Severity: req.Severity,
		Label:    req.Label,
		Enabled:  *req.Enabled,
		Priority: req.Priority,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to upsert marker pattern", nil)
		return
	}

	patterns, err := h.repo.ListMarkerPatterns(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list marker patterns", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyPatterns: patterns})
}

func (h *Handler) deleteMarkerPattern(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	patternID := strings.TrimSpace(r.PathValue("pattern"))
	if patternID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "pattern id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.DeleteMarkerPattern(ctx, patternID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "marker pattern not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete marker pattern", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyRemoved: patternID})
}
