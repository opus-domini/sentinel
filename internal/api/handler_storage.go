package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) opsConfig(w http.ResponseWriter, _ *http.Request) {
	if h.configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "CONFIG_UNAVAILABLE", "config path not set", nil)
		return
	}
	h.configMu.Lock()
	content, err := os.ReadFile(h.configPath)
	h.configMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_READ_FAILED", "failed to read config file", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"path":    h.configPath,
		"content": redactServerToken(string(content)),
	})
}

var tomlTableRE = regexp.MustCompile(`^\s*\[([^\]]+)\]`)

func redactServerToken(content string) string {
	lines := strings.SplitAfter(content, "\n")
	inServer := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if m := tomlTableRE.FindStringSubmatch(trimmed); m != nil {
			inServer = strings.TrimSpace(m[1]) == "server"
			continue
		}
		if !inServer || strings.HasPrefix(trimmed, "#") {
			continue
		}
		prefix := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		body := strings.TrimLeft(line, " \t")
		rest, ok := strings.CutPrefix(body, "token")
		if ok && strings.HasPrefix(strings.TrimSpace(rest), "=") {
			newline := ""
			if strings.HasSuffix(line, "\n") {
				newline = "\n"
			}
			lines[i] = prefix + `token = "[REDACTED]"` + newline
		}
	}
	return strings.Join(lines, "")
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
	h.configMu.Lock()
	err := os.WriteFile(h.configPath, []byte(req.Content), 0o600)
	h.configMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to write config file", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"path":     h.configPath,
		keyMessage: "config updated (restart required for changes to take effect)",
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
		h.configMu.Lock()
		err := upsertConfigString(h.configPath, "server", "timezone", tz)
		h.configMu.Unlock()
		if err != nil {
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
		h.configMu.Lock()
		err := upsertConfigString(h.configPath, "server", "locale", loc)
		h.configMu.Unlock()
		if err != nil {
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

func (h *Handler) getMCPSettings(w http.ResponseWriter, _ *http.Request) {
	if h.mcpSettings == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "MCP settings are unavailable", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"enabled":         h.mcpSettings.Enabled(),
		"tokenConfigured": h.mcpSettings.TokenConfigured(),
		"endpoint":        "/mcp",
	})
}

func (h *Handler) patchMCPSettings(w http.ResponseWriter, r *http.Request) {
	if h.mcpSettings == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "MCP settings are unavailable", nil)
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "enabled is required", nil)
		return
	}
	if *req.Enabled && !h.mcpSettings.TokenConfigured() {
		writeError(w, http.StatusConflict, "MCP_TOKEN_REQUIRED", "configure server.token before enabling MCP", nil)
		return
	}

	if h.configPath != "" {
		h.configMu.Lock()
		err := upsertConfigValue(h.configPath, "mcp", "enabled", strconv.FormatBool(*req.Enabled))
		h.configMu.Unlock()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to persist MCP setting", nil)
			return
		}
	}
	if err := h.mcpSettings.SetEnabled(*req.Enabled); err != nil {
		writeError(w, http.StatusConflict, "MCP_TOKEN_REQUIRED", err.Error(), nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"enabled":         h.mcpSettings.Enabled(),
		"tokenConfigured": h.mcpSettings.TokenConfigured(),
		"endpoint":        "/mcp",
	})
}

func upsertConfigString(path, section, key, value string) error {
	return upsertConfigValue(path, section, key, strconv.Quote(value))
}

// upsertConfigValue updates or inserts a sectioned key in the config file.
func upsertConfigValue(path, section, key, encodedValue string) error {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from DataDir
	if err != nil {
		return err
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	found := false
	inSection := false
	sectionSeen := false
	sectionHeader := "[" + section + "]"
	newLine := "  " + key + " = " + encodedValue
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inSection && !found {
				lines = append(lines, newLine)
				found = true
			}
			inSection = trimmed == sectionHeader
			if inSection {
				sectionSeen = true
			}
		}
		if inSection && !found && matchesConfigKey(trimmed, key) {
			lines = append(lines, newLine)
			found = true
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !found {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		if !sectionSeen {
			lines = append(lines, sectionHeader)
		}
		lines = append(lines, newLine)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return config.ValidateFile(path)
}

func matchesConfigKey(trimmed, key string) bool {
	return strings.HasPrefix(trimmed, key+" =") ||
		strings.HasPrefix(trimmed, key+"=") ||
		strings.HasPrefix(trimmed, "# "+key+" =") ||
		strings.HasPrefix(trimmed, "# "+key+"=")
}
