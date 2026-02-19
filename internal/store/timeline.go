package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	opsSeverityInfo  = "info"
	opsSeverityWarn  = "warn"
	opsSeverityError = "error"

	opsDefaultSource = "ops"
)

var ErrInvalidOpsFilter = errors.New("invalid ops filter")

type OpsTimelineEvent struct {
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

type OpsTimelineEventWrite struct {
	Source    string
	EventType string
	Severity  string
	Resource  string
	Message   string
	Details   string
	Metadata  string
	CreatedAt time.Time
}

type OpsTimelineQuery struct {
	Query    string
	Severity string
	Source   string
	Limit    int
}

type OpsTimelineResult struct {
	Events  []OpsTimelineEvent `json:"events"`
	HasMore bool               `json:"hasMore"`
}

func (s *Store) initTimelineSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ops_timeline_events (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			source       TEXT NOT NULL,
			event_type   TEXT NOT NULL,
			severity     TEXT NOT NULL,
			resource     TEXT NOT NULL,
			message      TEXT NOT NULL,
			details      TEXT NOT NULL DEFAULT '',
			metadata     TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_timeline_created
			ON ops_timeline_events (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_timeline_severity
			ON ops_timeline_events (severity, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_timeline_source
			ON ops_timeline_events (source, created_at DESC, id DESC)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertOpsTimelineEvent(ctx context.Context, write OpsTimelineEventWrite) (OpsTimelineEvent, error) {
	now := write.CreatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	source := strings.TrimSpace(write.Source)
	if source == "" {
		source = opsDefaultSource
	}
	eventType := strings.TrimSpace(write.EventType)
	if eventType == "" {
		eventType = "ops.event"
	}
	severity := normalizeOpsSeverity(write.Severity)

	res, err := s.db.ExecContext(ctx, `INSERT INTO ops_timeline_events (
		source, event_type, severity, resource, message, details, metadata, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		source,
		eventType,
		severity,
		strings.TrimSpace(write.Resource),
		strings.TrimSpace(write.Message),
		strings.TrimSpace(write.Details),
		strings.TrimSpace(write.Metadata),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return OpsTimelineEvent{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return OpsTimelineEvent{}, err
	}
	return s.getOpsTimelineEventByID(ctx, id)
}

func (s *Store) getOpsTimelineEventByID(ctx context.Context, id int64) (OpsTimelineEvent, error) {
	var out OpsTimelineEvent
	err := s.db.QueryRowContext(ctx, `SELECT
		id, source, event_type, severity, resource, message, details, metadata, created_at
	FROM ops_timeline_events
	WHERE id = ?`, id).Scan(
		&out.ID,
		&out.Source,
		&out.EventType,
		&out.Severity,
		&out.Resource,
		&out.Message,
		&out.Details,
		&out.Metadata,
		&out.CreatedAt,
	)
	if err != nil {
		return OpsTimelineEvent{}, err
	}
	return out, nil
}

func (s *Store) SearchOpsTimelineEvents(ctx context.Context, query OpsTimelineQuery) (OpsTimelineResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	search := "%" + strings.ToLower(strings.TrimSpace(query.Query)) + "%"
	rawSeverity := strings.ToLower(strings.TrimSpace(query.Severity))
	severity := ""
	switch rawSeverity {
	case "", "all":
		severity = ""
	case opsSeverityInfo, opsSeverityWarn, "warning", opsSeverityError, "err":
		severity = normalizeOpsSeverity(rawSeverity)
	default:
		return OpsTimelineResult{}, fmt.Errorf("%w: severity", ErrInvalidOpsFilter)
	}
	source := strings.ToLower(strings.TrimSpace(query.Source))

	rows, err := s.db.QueryContext(ctx, `SELECT
		id, source, event_type, severity, resource, message, details, metadata, created_at
	FROM ops_timeline_events
	WHERE (? = '' OR severity = ?)
	  AND (? = '' OR lower(source) = ?)
	  AND (? = '%%' OR (
		lower(message) LIKE ? OR
		lower(details) LIKE ? OR
		lower(resource) LIKE ? OR
		lower(event_type) LIKE ?
	  ))
	ORDER BY created_at DESC, id DESC
	LIMIT ?`, severity, severity, source, source, search, search, search, search, search, limit+1)
	if err != nil {
		return OpsTimelineResult{}, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]OpsTimelineEvent, 0, limit+1)
	for rows.Next() {
		var item OpsTimelineEvent
		if err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.EventType,
			&item.Severity,
			&item.Resource,
			&item.Message,
			&item.Details,
			&item.Metadata,
			&item.CreatedAt,
		); err != nil {
			return OpsTimelineResult{}, err
		}
		events = append(events, item)
	}
	if err := rows.Err(); err != nil {
		return OpsTimelineResult{}, err
	}

	result := OpsTimelineResult{Events: events}
	if len(result.Events) > limit {
		result.HasMore = true
		result.Events = result.Events[:limit]
	}
	return result, nil
}

func normalizeOpsSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case opsSeverityWarn, "warning":
		return opsSeverityWarn
	case opsSeverityError, "err":
		return opsSeverityError
	default:
		return opsSeverityInfo
	}
}
