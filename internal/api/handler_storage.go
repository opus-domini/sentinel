package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) opsConfig(w http.ResponseWriter, _ *http.Request) {
	if h.configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "CONFIG_UNAVAILABLE", "config path not set", nil)
		return
	}
	content, err := os.ReadFile(h.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_READ_FAILED", "failed to read config file", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"path":    h.configPath,
		"content": string(content),
	})
}

func (h *Handler) patchOpsConfig(w http.ResponseWriter, r *http.Request) {
	if h.configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "CONFIG_UNAVAILABLE", "config path not set", nil)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "content is required", nil)
		return
	}
	if err := os.WriteFile(h.configPath, []byte(req.Content), 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to write config file", nil)
		return
	}

	now := time.Now().UTC()
	te, _ := h.orch.RecordConfigUpdated(r.Context(), now)
	if te.ID > 0 {
		h.emit(events.TypeOpsActivity, map[string]any{
			"globalRev": now.UnixMilli(),
			"event":     te,
		})
	}

	writeData(w, http.StatusOK, map[string]any{
		"path":    h.configPath,
		"message": "config updated (restart required for changes to take effect)",
	})
}

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

func (h *Handler) patchTimezone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Timezone string `json:"timezone"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "timezone is required", nil)
		return
	}
	if err := validate.Timezone(tz); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	if h.configPath != "" {
		if err := upsertConfigKey(h.configPath, "timezone", tz); err != nil {
			writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to persist timezone", nil)
			return
		}
	}

	h.mu.Lock()
	h.timezone = tz
	h.mu.Unlock()

	writeData(w, http.StatusOK, map[string]any{
		"timezone": tz,
	})
}

func (h *Handler) patchLocale(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Locale string `json:"locale"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	loc := strings.TrimSpace(req.Locale)
	// Empty locale is valid — means "use browser default".

	if h.configPath != "" {
		if err := upsertConfigKey(h.configPath, "locale", loc); err != nil {
			writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to persist locale", nil)
			return
		}
	}

	h.mu.Lock()
	h.locale = loc
	h.mu.Unlock()

	writeData(w, http.StatusOK, map[string]any{
		"locale": loc,
	})
}

// upsertConfigKey updates or inserts a key = "value" line in the config file.
func upsertConfigKey(path, key, value string) error {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from DataDir
	if err != nil {
		return err
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	found := false
	newLine := key + ` = "` + value + `"`
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// Match both active and commented-out lines for this key.
		if !found && (strings.HasPrefix(trimmed, key+" =") || strings.HasPrefix(trimmed, key+"=") ||
			strings.HasPrefix(trimmed, "# "+key+" =") || strings.HasPrefix(trimmed, "# "+key+"=")) {
			lines = append(lines, newLine)
			found = true
			continue
		}
		lines = append(lines, line)
	}
	if !found {
		lines = append(lines, newLine)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600) //nolint:gosec // fixed content
}
