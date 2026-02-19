package watchtower

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func normalizePaneTail(captured string) string {
	captured = strings.TrimSpace(captured)
	if captured == "" {
		return ""
	}
	lines := strings.Split(captured, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) > 4 {
		filtered = filtered[len(filtered)-4:]
	}
	return strings.Join(filtered, "\n")
}

func hashPaneTail(normalized string) string {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum[:8])
}
