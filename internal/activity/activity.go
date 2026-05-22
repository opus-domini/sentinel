// Package activity tracks operational activity events.
package activity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Severity levels for activity events.
const (
	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityError = "error"
)

// Sources identify the subsystem that produced an activity event.
const (
	DefaultSource   = "ops"
	SourceService   = "service"
	SourceAlert     = "alert"
	SourceRunbook   = "runbook"
	SourceSchedule  = "schedule"
	SourceConfig    = "config"
	SourceGuardrail = "guardrail"
)

// Event types recorded on the activity timeline.
const (
	EventServiceAction       = "service.action"
	EventServiceRegistered   = "service.registered"
	EventServiceUnregistered = "service.unregistered"
	EventAlertCreated        = "alert.created"
	EventAlertAcked          = "alert.acked"
	EventAlertResolved       = "alert.resolved"
	EventAlertDeleted        = "alert.deleted"
	EventRunbookStarted      = "runbook.started"
	EventConfigUpdated       = "config.updated"
	EventGuardrailBlocked    = "guardrail.blocked"
	EventScheduleCreated     = "schedule.created"
	EventScheduleTriggered   = "schedule.triggered"
	EventScheduleDeleted     = "schedule.deleted"
)

// ErrInvalidFilter is returned when a filter value (e.g. severity) is not recognized.
var ErrInvalidFilter = errors.New("invalid activity filter")

// Event represents a recorded activity event.
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

// EventWrite contains the fields needed to create an activity event.
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

// Query specifies search parameters for activity events.
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

// Repo defines the persistence operations consumed by the activity service.
type Repo interface {
	InsertActivityEvent(ctx context.Context, write EventWrite) (Event, error)
	SearchActivityEvents(ctx context.Context, query Query) (Result, error)
}
