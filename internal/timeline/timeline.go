package timeline

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Severity levels for timeline events.
const (
	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityError = "error"

	DefaultSource = "ops"
)

// ErrInvalidFilter is returned when a filter value (e.g. severity) is not recognized.
var ErrInvalidFilter = errors.New("invalid timeline filter")

// Event represents a recorded timeline event.
type Event struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	EventType string `json:"eventType"`
	Severity  string `json:"severity"`
	Resource  string `json:"resource"`
	Message   string `json:"message"`
	Details   string `json:"details"`
	Metadata  string `json:"metadata"`
	CreatedAt string `json:"createdAt"`
}

// EventWrite contains the fields needed to create a timeline event.
type EventWrite struct {
	Source    string
	EventType string
	Severity  string
	Resource  string
	Message   string
	Details   string
	Metadata  string
	CreatedAt time.Time
}

// Query specifies search parameters for timeline events.
type Query struct {
	Query    string
	Severity string
	Source   string
	Limit    int
}

// Result contains the events returned from a search plus pagination info.
type Result struct {
	Events  []Event
	HasMore bool
}

// NormalizeSeverity maps common severity aliases to canonical values.
// Unknown values are returned as-is for the caller to validate.
func NormalizeSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return SeverityInfo
	case "warning":
		return SeverityWarn
	case "err":
		return SeverityError
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// Repo defines the persistence operations consumed by the timeline service.
type Repo interface {
	InsertTimelineEvent(ctx context.Context, write EventWrite) (Event, error)
	SearchTimelineEvents(ctx context.Context, query Query) (Result, error)
}
