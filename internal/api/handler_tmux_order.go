package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) reorderSessions(w http.ResponseWriter, r *http.Request) {
	names, err := decodeSessionOrderNames(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.ReorderSessions(ctx, names); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder sessions", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reorderSessionPresets(w http.ResponseWriter, r *http.Request) {
	names, err := decodeSessionOrderNames(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.ReorderSessionPresets(ctx, names); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder session presets", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeSessionOrderNames(r *http.Request) ([]string, error) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return nil, err
	}
	if len(req.Names) == 0 {
		return nil, errors.New("names are required")
	}
	seen := make(map[string]struct{}, len(req.Names))
	for _, name := range req.Names {
		if !validate.SessionName(name) {
			return nil, errors.New("names must match ^[A-Za-z0-9._-]{1,64}$")
		}
		if _, ok := seen[name]; ok {
			return nil, errors.New("names must be unique")
		}
		seen[name] = struct{}{}
	}
	return req.Names, nil
}
