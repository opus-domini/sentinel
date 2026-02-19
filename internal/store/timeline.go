package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/timeline"
)

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

func (s *Store) InsertTimelineEvent(ctx context.Context, write timeline.EventWrite) (timeline.Event, error) {
	now := write.CreatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	source := strings.TrimSpace(write.Source)
	if source == "" {
		source = timeline.DefaultSource
	}
	eventType := strings.TrimSpace(write.EventType)
	if eventType == "" {
		eventType = "ops.event"
	}
	severity := timeline.NormalizeSeverity(write.Severity)

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
		return timeline.Event{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return timeline.Event{}, err
	}
	return s.getTimelineEventByID(ctx, id)
}

func (s *Store) getTimelineEventByID(ctx context.Context, id int64) (timeline.Event, error) {
	var out timeline.Event
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
		return timeline.Event{}, err
	}
	return out, nil
}

func (s *Store) SearchTimelineEvents(ctx context.Context, query timeline.Query) (timeline.Result, error) {
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
	case timeline.SeverityInfo, timeline.SeverityWarn, "warning", timeline.SeverityError, "err":
		severity = timeline.NormalizeSeverity(rawSeverity)
	default:
		return timeline.Result{}, fmt.Errorf("%w: severity", timeline.ErrInvalidFilter)
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
		return timeline.Result{}, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]timeline.Event, 0, limit+1)
	for rows.Next() {
		var item timeline.Event
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
			return timeline.Result{}, err
		}
		events = append(events, item)
	}
	if err := rows.Err(); err != nil {
		return timeline.Result{}, err
	}

	result := timeline.Result{Events: events}
	if len(result.Events) > limit {
		result.HasMore = true
		result.Events = result.Events[:limit]
	}
	return result, nil
}
