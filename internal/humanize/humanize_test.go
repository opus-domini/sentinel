package humanize

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value int64
		want  string
	}{
		{-1, "-"},
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
	}
	for _, tt := range tests {
		if got := Bytes(tt.value); got != tt.want {
			t.Fatalf("Bytes(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value time.Duration
		want  string
	}{
		{-time.Second, "-"},
		{0, "0s"},
		{150 * time.Millisecond, "150ms"},
		{time.Second, "1s"},
		{2 * time.Minute, "2m"},
		{3 * time.Hour, "3h"},
		{time.Hour + 30*time.Minute, "1h30m0s"},
	}
	for _, tt := range tests {
		if got := Duration(tt.value); got != tt.want {
			t.Fatalf("Duration(%s) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ratio  float64
		digits int
		want   string
	}{
		{0, 0, "0%"},
		{0.5, 0, "50%"},
		{0.552, 1, "55.2%"},
		{0.552, 99, "55.200%"},
		{-1, 1, "-"},
	}
	for _, tt := range tests {
		if got := Percent(tt.ratio, tt.digits); got != tt.want {
			t.Fatalf("Percent(%f, %d) = %q, want %q", tt.ratio, tt.digits, got, tt.want)
		}
	}
}

func TestPluralize(t *testing.T) {
	t.Parallel()

	if got := Pluralize(1, "row", "rows"); got != "row" {
		t.Fatalf("Pluralize singular = %q", got)
	}
	if got := Pluralize(2, "row", ""); got != "rows" {
		t.Fatalf("Pluralize plural = %q", got)
	}
}

func TestTimeAndValueOrDash(t *testing.T) {
	t.Parallel()

	if got := Time(time.Time{}); got != "-" {
		t.Fatalf("Time(zero) = %q", got)
	}
	stamp := time.Date(2026, 5, 27, 12, 30, 0, 0, time.FixedZone("BRT", -3*3600))
	if got := Time(stamp); got != "2026-05-27T15:30:00Z" {
		t.Fatalf("Time(stamp) = %q", got)
	}
	if got := ValueOrDash("  "); got != "-" {
		t.Fatalf("ValueOrDash(empty) = %q", got)
	}
	if got := ValueOrDash("x"); got != "x" {
		t.Fatalf("ValueOrDash(value) = %q", got)
	}
}
