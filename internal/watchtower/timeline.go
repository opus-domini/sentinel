package watchtower

import (
	"encoding/json"
	"strings"
)

var (
	errorMarkers = []string{
		"panic",
		"fatal",
		"segmentation fault",
		"traceback",
		"exception",
		"xdebug",
		"permission denied",
		"error",
		"failed",
	}
	warnMarkers = []string{
		"warning",
		"warn",
		"deprecated",
		"timeout",
		"retry",
		"slow",
	}
)

func normalizeRuntimeCommand(current, start string) string {
	command := strings.TrimSpace(current)
	if command == "" {
		command = strings.TrimSpace(start)
	}
	if command == "-" {
		return ""
	}
	return command
}

func isShellLikeCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	switch command {
	case "", "sh", "bash", "zsh", "fish", "tmux":
		return true
	default:
		return false
	}
}

func detectTimelineMarker(preview string) (string, string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(preview))
	if normalized == "" {
		return "", "", false
	}

	for _, marker := range errorMarkers {
		if strings.Contains(normalized, marker) {
			return marker, "error", true
		}
	}
	for _, marker := range warnMarkers {
		if strings.Contains(normalized, marker) {
			return marker, "warn", true
		}
	}
	return "", "", false
}

func timelineLastLine(preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return ""
	}
	lines := strings.Split(preview, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if len(last) > 240 {
		return last[:240]
	}
	return last
}

func timelineMetadataJSON(values map[string]any) json.RawMessage {
	if values == nil {
		return json.RawMessage("{}")
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(payload)
}
