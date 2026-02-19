package alerts

import (
	"context"
	"errors"
	"time"
)

// Status constants for alert lifecycle.
const (
	StatusOpen     = "open"
	StatusAcked    = "acked"
	StatusResolved = "resolved"
)

// ErrInvalidFilter is returned when a filter value (e.g. status) is not recognized.
var ErrInvalidFilter = errors.New("invalid alerts filter")

// Alert represents a persisted alert with deduplication support.
type Alert struct {
	ID          int64  `json:"id"`
	DedupeKey   string `json:"dedupeKey"`
	Source      string `json:"source"`
	Resource    string `json:"resource"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Occurrences int64  `json:"occurrences"`
	Metadata    string `json:"metadata"`
	FirstSeenAt string `json:"firstSeenAt"`
	LastSeenAt  string `json:"lastSeenAt"`
	AckedAt     string `json:"ackedAt,omitempty"`
	ResolvedAt  string `json:"resolvedAt,omitempty"`
}

// AlertWrite contains the fields needed to create or update an alert.
type AlertWrite struct {
	DedupeKey string
	Source    string
	Resource  string
	Title     string
	Message   string
	Severity  string
	Metadata  string
	CreatedAt time.Time
}

// Repo defines the persistence operations consumed by the alerts service.
type Repo interface {
	UpsertAlert(ctx context.Context, write AlertWrite) (Alert, error)
	AckAlert(ctx context.Context, id int64, at time.Time) (Alert, error)
	ListAlerts(ctx context.Context, limit int, status string) ([]Alert, error)
	ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (Alert, error)
	DeleteAlert(ctx context.Context, id int64) error
}
