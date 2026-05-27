// Package humanize turns machine values into compact human-readable strings.
package humanize

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Bytes formats a byte count using binary units.
func Bytes(value int64) string {
	if value < 0 {
		return "-"
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

// Duration formats a duration as a compact value that remains parseable by Go.
func Duration(value time.Duration) string {
	if value < 0 {
		return "-"
	}
	if value == 0 {
		return "0s"
	}
	if value%time.Hour == 0 {
		return fmt.Sprintf("%dh", value/time.Hour)
	}
	if value > time.Hour {
		return value.String()
	}
	if value%time.Minute == 0 {
		return fmt.Sprintf("%dm", value/time.Minute)
	}
	if value%time.Second == 0 {
		return fmt.Sprintf("%ds", value/time.Second)
	}
	return value.String()
}

// Percent formats a 0-1 ratio as a percent string.
func Percent(ratio float64, digits int) string {
	if math.IsNaN(ratio) || ratio < 0 {
		return "-"
	}
	if digits < 0 {
		digits = 0
	}
	if digits > 3 {
		digits = 3
	}
	return fmt.Sprintf("%.*f%%", digits, ratio*100)
}

// Pluralize returns singular when count is one and plural otherwise.
func Pluralize(count int64, singular, plural string) string {
	if count == 1 {
		return singular
	}
	if plural != "" {
		return plural
	}
	return singular + "s"
}

// Time formats a timestamp for CLI output.
func Time(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

// ValueOrDash returns "-" for empty strings.
func ValueOrDash(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "-"
	}
	return raw
}
