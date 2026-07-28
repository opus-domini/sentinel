package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

func (h *Handler) storageStats(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := h.repo.GetStorageStats(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load storage stats", nil)
		return
	}
	writeData(w, http.StatusOK, stats)
}

func (h *Handler) flushStorage(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	var req struct {
		Resource string `json:"resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	resource := store.NormalizeStorageResource(req.Resource)
	if resource == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "resource is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	results, err := h.repo.FlushStorageResource(ctx, resource)
	if err != nil {
		if errors.Is(err, store.ErrInvalidStorageResource) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to flush storage resource", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"results":   results,
		"flushedAt": time.Now().UTC().Format(time.RFC3339),
	})
}
