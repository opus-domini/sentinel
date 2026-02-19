package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	timelineSeverityInfo  = "info"
	timelineSeverityWarn  = "warn"
	timelineSeverityError = "error"
)

type WatchtowerTimelineEvent struct {
	ID         int64           `json:"id"`
	Session    string          `json:"session"`
	WindowIdx  int             `json:"windowIndex"`
	PaneID     string          `json:"paneId"`
	EventType  string          `json:"eventType"`
	Severity   string          `json:"severity"`
	Command    string          `json:"command"`
	Cwd        string          `json:"cwd"`
	DurationMS int64           `json:"durationMs"`
	Summary    string          `json:"summary"`
	Details    string          `json:"details"`
	Marker     string          `json:"marker"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type WatchtowerTimelineEventWrite struct {
	Session    string
	WindowIdx  int
	PaneID     string
	EventType  string
	Severity   string
	Command    string
	Cwd        string
	DurationMS int64
	Summary    string
	Details    string
	Marker     string
	Metadata   json.RawMessage
	CreatedAt  time.Time
}

type WatchtowerTimelineQuery struct {
	Session   string
	WindowIdx *int
	PaneID    string
	Query     string
	Severity  string
	EventType string
	Since     time.Time
	Until     time.Time
	Limit     int
}

type WatchtowerTimelineResult struct {
	Events  []WatchtowerTimelineEvent `json:"events"`
	HasMore bool                      `json:"hasMore"`
}

type WatchtowerPaneRuntime struct {
	PaneID         string
	SessionName    string
	WindowIdx      int
	CurrentCommand string
	StartedAt      time.Time
	UpdatedAt      time.Time
}

type WatchtowerPaneRuntimeWrite struct {
	PaneID         string
	SessionName    string
	WindowIdx      int
	CurrentCommand string
	StartedAt      time.Time
	UpdatedAt      time.Time
}

func (s *Store) InsertWatchtowerTimelineEvent(ctx context.Context, row WatchtowerTimelineEventWrite) (int64, error) {
	eventType := strings.TrimSpace(row.EventType)
	if eventType == "" {
		return 0, errors.New("event type is required")
	}

	createdAt := row.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	metadata := []byte("{}")
	if len(row.Metadata) > 0 {
		metadata = row.Metadata
	}
	durationMS := row.DurationMS
	if durationMS < 0 {
		durationMS = 0
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_timeline_events (
			session_name, window_index, pane_id, event_type, severity,
			command, cwd, duration_ms, summary, details, marker, metadata_json, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(row.Session),
		row.WindowIdx,
		strings.TrimSpace(row.PaneID),
		eventType,
		normalizeTimelineSeverity(row.Severity),
		strings.TrimSpace(row.Command),
		strings.TrimSpace(row.Cwd),
		durationMS,
		strings.TrimSpace(row.Summary),
		strings.TrimSpace(row.Details),
		strings.TrimSpace(row.Marker),
		string(metadata),
		createdAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) SearchWatchtowerTimelineEvents(ctx context.Context, query WatchtowerTimelineQuery) (WatchtowerTimelineResult, error) {
	limit := normalizeTimelineLimit(query.Limit)
	sqlQuery, args := buildTimelineSearchQuery(query, limit)
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return WatchtowerTimelineResult{}, err
	}
	defer func() { _ = rows.Close() }()

	events, err := scanTimelineRows(rows, limit)
	if err != nil {
		return WatchtowerTimelineResult{}, err
	}

	hasMore := false
	if len(events) > limit {
		hasMore = true
		events = events[:limit]
	}
	return WatchtowerTimelineResult{
		Events:  events,
		HasMore: hasMore,
	}, nil
}

func normalizeTimelineLimit(raw int) int {
	limit := raw
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

func buildTimelineSearchQuery(query WatchtowerTimelineQuery, limit int) (string, []any) {
	clauses, args := buildTimelineSearchFilters(query)
	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`SELECT id, session_name, window_index, pane_id, event_type, severity,
		command, cwd, duration_ms, summary, details, marker, metadata_json, created_at
		FROM wt_timeline_events`)
	if len(clauses) > 0 {
		sqlBuilder.WriteString(" WHERE ")
		sqlBuilder.WriteString(strings.Join(clauses, " AND "))
	}
	sqlBuilder.WriteString(" ORDER BY created_at DESC, id DESC LIMIT ?")
	args = append(args, limit+1)
	return sqlBuilder.String(), args
}

func buildTimelineSearchFilters(query WatchtowerTimelineQuery) ([]string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 16)
	if session := strings.TrimSpace(query.Session); session != "" {
		appendTimelineFilter(&clauses, &args, "session_name = ?", session)
	}
	if query.WindowIdx != nil {
		appendTimelineFilter(&clauses, &args, "window_index = ?", *query.WindowIdx)
	}
	if paneID := strings.TrimSpace(query.PaneID); paneID != "" {
		appendTimelineFilter(&clauses, &args, "pane_id = ?", paneID)
	}
	if severity := normalizeTimelineSeverity(query.Severity); severity != "" {
		appendTimelineFilter(&clauses, &args, "severity = ?", severity)
	}
	if eventType := strings.TrimSpace(query.EventType); eventType != "" {
		appendTimelineFilter(&clauses, &args, "event_type = ?", eventType)
	}
	if !query.Since.IsZero() {
		appendTimelineFilter(&clauses, &args, "created_at >= ?", query.Since.UTC().Format(time.RFC3339))
	}
	if !query.Until.IsZero() {
		appendTimelineFilter(&clauses, &args, "created_at <= ?", query.Until.UTC().Format(time.RFC3339))
	}
	appendTimelineTextQuery(&clauses, &args, query.Query)
	return clauses, args
}

func appendTimelineFilter(clauses *[]string, args *[]any, clause string, value any) {
	*clauses = append(*clauses, clause)
	*args = append(*args, value)
}

func appendTimelineTextQuery(clauses *[]string, args *[]any, raw string) {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return
	}
	like := "%" + needle + "%"
	*clauses = append(*clauses, "(lower(summary) LIKE ? OR lower(details) LIKE ? OR lower(command) LIKE ? OR lower(cwd) LIKE ? OR lower(marker) LIKE ?)")
	*args = append(*args, like, like, like, like, like)
}

func scanTimelineRows(rows *sql.Rows, limit int) ([]WatchtowerTimelineEvent, error) {
	events := make([]WatchtowerTimelineEvent, 0, limit+1)
	for rows.Next() {
		row, err := scanTimelineRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func scanTimelineRow(rows *sql.Rows) (WatchtowerTimelineEvent, error) {
	var (
		row          WatchtowerTimelineEvent
		metadataRaw  string
		createdAtRaw string
	)
	if err := rows.Scan(
		&row.ID,
		&row.Session,
		&row.WindowIdx,
		&row.PaneID,
		&row.EventType,
		&row.Severity,
		&row.Command,
		&row.Cwd,
		&row.DurationMS,
		&row.Summary,
		&row.Details,
		&row.Marker,
		&metadataRaw,
		&createdAtRaw,
	); err != nil {
		return WatchtowerTimelineEvent{}, err
	}
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.Metadata = normalizeTimelineMetadata(metadataRaw)
	return row, nil
}

func normalizeTimelineMetadata(raw string) json.RawMessage {
	metadata := json.RawMessage(strings.TrimSpace(raw))
	if len(metadata) == 0 {
		return json.RawMessage("{}")
	}
	return metadata
}

func (s *Store) PruneWatchtowerTimelineRows(ctx context.Context, maxRows int) (int64, error) {
	if maxRows <= 0 {
		return 0, nil
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wt_timeline_events
		  WHERE id IN (
			SELECT id
			FROM wt_timeline_events
			ORDER BY created_at DESC, id DESC
			LIMIT -1 OFFSET ?
		  )`,
		maxRows,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) UpsertWatchtowerPaneRuntime(ctx context.Context, row WatchtowerPaneRuntimeWrite) error {
	paneID := strings.TrimSpace(row.PaneID)
	if paneID == "" {
		return errors.New("pane id is required")
	}
	sessionName := strings.TrimSpace(row.SessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_pane_runtime (
			pane_id, session_name, window_index, current_command, started_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pane_id) DO UPDATE SET
			session_name = excluded.session_name,
			window_index = excluded.window_index,
			current_command = excluded.current_command,
			started_at = excluded.started_at,
			updated_at = excluded.updated_at`,
		paneID,
		sessionName,
		row.WindowIdx,
		strings.TrimSpace(row.CurrentCommand),
		formatStoreValueTime(row.StartedAt),
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListWatchtowerPaneRuntimeBySession(ctx context.Context, sessionName string) ([]WatchtowerPaneRuntime, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return []WatchtowerPaneRuntime{}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT pane_id, session_name, window_index, current_command, started_at, updated_at
		   FROM wt_pane_runtime
		  WHERE session_name = ?
		  ORDER BY pane_id ASC`,
		sessionName,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPaneRuntime, 0, 16)
	for rows.Next() {
		var (
			row                   WatchtowerPaneRuntime
			startedAtRaw, updated string
		)
		if err := rows.Scan(
			&row.PaneID,
			&row.SessionName,
			&row.WindowIdx,
			&row.CurrentCommand,
			&startedAtRaw,
			&updated,
		); err != nil {
			return nil, err
		}
		row.StartedAt = parseStoreTime(startedAtRaw)
		row.UpdatedAt = parseStoreTime(updated)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PurgeWatchtowerPaneRuntime(ctx context.Context, sessionName string, activePaneIDs []string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}
	if len(activePaneIDs) == 0 {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM wt_pane_runtime WHERE session_name = ?",
			sessionName,
		)
		return err
	}

	placeholders := sqlPlaceholders(len(activePaneIDs))
	args := make([]any, 0, len(activePaneIDs)+1)
	args = append(args, sessionName)
	args = append(args, stringsToAny(activePaneIDs)...)
	query := "DELETE FROM wt_pane_runtime WHERE session_name = ? AND pane_id NOT IN (" + placeholders + ")" //nolint:gosec // placeholders are generated literals
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func normalizeTimelineSeverity(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case timelineSeverityInfo, timelineSeverityWarn, timelineSeverityError:
		return normalized
	default:
		return ""
	}
}
