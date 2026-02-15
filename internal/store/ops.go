package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	opsAlertStatusOpen     = "open"
	opsAlertStatusAcked    = "acked"
	opsAlertStatusResolved = "resolved"

	opsSeverityInfo  = "info"
	opsSeverityWarn  = "warn"
	opsSeverityError = "error"

	opsRunbookStatusQueued    = "queued"
	opsRunbookStatusRunning   = "running"
	opsRunbookStatusSucceeded = "succeeded"
	opsRunbookStatusFailed    = "failed"
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

type OpsAlert struct {
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

type OpsAlertWrite struct {
	DedupeKey string
	Source    string
	Resource  string
	Title     string
	Message   string
	Severity  string
	Metadata  string
	CreatedAt time.Time
}

type OpsRunbookStep struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Command     string `json:"command,omitempty"`
	Check       string `json:"check,omitempty"`
	Description string `json:"description,omitempty"`
}

type OpsRunbook struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Enabled     bool             `json:"enabled"`
	Steps       []OpsRunbookStep `json:"steps"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
}

type OpsRunbookRun struct {
	ID             string `json:"id"`
	RunbookID      string `json:"runbookId"`
	RunbookName    string `json:"runbookName"`
	Status         string `json:"status"`
	TotalSteps     int    `json:"totalSteps"`
	CompletedSteps int    `json:"completedSteps"`
	CurrentStep    string `json:"currentStep"`
	Error          string `json:"error"`
	CreatedAt      string `json:"createdAt"`
	StartedAt      string `json:"startedAt,omitempty"`
	FinishedAt     string `json:"finishedAt,omitempty"`
}

func (s *Store) initOpsSchema() error {
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
		`CREATE TABLE IF NOT EXISTS ops_alerts (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			dedupe_key    TEXT NOT NULL UNIQUE,
			source        TEXT NOT NULL,
			resource      TEXT NOT NULL,
			title         TEXT NOT NULL,
			message       TEXT NOT NULL,
			severity      TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'open',
			occurrences   INTEGER NOT NULL DEFAULT 1,
			metadata      TEXT NOT NULL DEFAULT '',
			first_seen_at TEXT NOT NULL,
			last_seen_at  TEXT NOT NULL,
			acked_at      TEXT DEFAULT '',
			resolved_at   TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_alerts_status
			ON ops_alerts (status, last_seen_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_alerts_last_seen
			ON ops_alerts (last_seen_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS ops_runbooks (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			steps_json  TEXT NOT NULL DEFAULT '[]',
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS ops_runbook_runs (
			id              TEXT PRIMARY KEY,
			runbook_id      TEXT NOT NULL,
			runbook_name    TEXT NOT NULL,
			status          TEXT NOT NULL,
			total_steps     INTEGER NOT NULL DEFAULT 0,
			completed_steps INTEGER NOT NULL DEFAULT 0,
			current_step    TEXT NOT NULL DEFAULT '',
			error           TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			started_at      TEXT NOT NULL DEFAULT '',
			finished_at     TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_created
			ON ops_runbook_runs (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_status
			ON ops_runbook_runs (status, created_at DESC, id DESC)`,
		`INSERT OR IGNORE INTO ops_runbooks(
			id, name, description, steps_json, enabled, created_at, updated_at
		) VALUES (
			'ops.service.recover',
			'Service Recovery',
			'Validate and recover the Sentinel service runtime.',
			'[{"type":"command","title":"Inspect service status","command":"sentinel service status"},{"type":"command","title":"Restart service","command":"sentinel service install --start=true"},{"type":"check","title":"Confirm healthy status","check":"service should be active"}]',
			1,
			datetime('now'),
			datetime('now')
		)`,
		`INSERT OR IGNORE INTO ops_runbooks(
			id, name, description, steps_json, enabled, created_at, updated_at
		) VALUES (
			'ops.autoupdate.verify',
			'Autoupdate Verification',
			'Check updater configuration and latest release state.',
			'[{"type":"command","title":"Check updater timer","command":"sentinel service autoupdate status"},{"type":"command","title":"Check release status","command":"sentinel update check"},{"type":"manual","title":"Review output","description":"Review versions and update policy before apply."}]',
			1,
			datetime('now'),
			datetime('now')
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
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
		source = "ops"
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

func (s *Store) UpsertOpsAlert(ctx context.Context, write OpsAlertWrite) (OpsAlert, error) {
	now := write.CreatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dedupeKey := strings.TrimSpace(write.DedupeKey)
	if dedupeKey == "" {
		return OpsAlert{}, fmt.Errorf("dedupe key is required")
	}
	source := strings.TrimSpace(write.Source)
	if source == "" {
		source = "ops"
	}
	resource := strings.TrimSpace(write.Resource)
	title := strings.TrimSpace(write.Title)
	if title == "" {
		title = dedupeKey
	}
	message := strings.TrimSpace(write.Message)
	if message == "" {
		message = title
	}
	severity := normalizeOpsSeverity(write.Severity)
	metadata := strings.TrimSpace(write.Metadata)
	nowRFC3339 := now.Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_alerts (
		dedupe_key, source, resource, title, message, severity, status, occurrences, metadata, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	ON CONFLICT(dedupe_key) DO UPDATE SET
		source = excluded.source,
		resource = excluded.resource,
		title = excluded.title,
		message = excluded.message,
		severity = excluded.severity,
		status = CASE WHEN ops_alerts.status = ? THEN ? ELSE ops_alerts.status END,
		occurrences = ops_alerts.occurrences + 1,
		metadata = excluded.metadata,
		last_seen_at = excluded.last_seen_at`,
		dedupeKey,
		source,
		resource,
		title,
		message,
		severity,
		opsAlertStatusOpen,
		metadata,
		nowRFC3339,
		nowRFC3339,
		opsAlertStatusResolved,
		opsAlertStatusOpen,
	); err != nil {
		return OpsAlert{}, err
	}

	return s.getOpsAlertByDedupeKey(ctx, dedupeKey)
}

func (s *Store) getOpsAlertByDedupeKey(ctx context.Context, dedupeKey string) (OpsAlert, error) {
	var out OpsAlert
	err := s.db.QueryRowContext(ctx, `SELECT
		id, dedupe_key, source, resource, title, message, severity, status, occurrences,
		metadata, first_seen_at, last_seen_at, acked_at, resolved_at
	FROM ops_alerts
	WHERE dedupe_key = ?`, dedupeKey).Scan(
		&out.ID,
		&out.DedupeKey,
		&out.Source,
		&out.Resource,
		&out.Title,
		&out.Message,
		&out.Severity,
		&out.Status,
		&out.Occurrences,
		&out.Metadata,
		&out.FirstSeenAt,
		&out.LastSeenAt,
		&out.AckedAt,
		&out.ResolvedAt,
	)
	if err != nil {
		return OpsAlert{}, err
	}
	return out, nil
}

func (s *Store) ListOpsAlerts(ctx context.Context, limit int, status string) ([]OpsAlert, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != opsAlertStatusOpen && status != opsAlertStatusAcked && status != opsAlertStatusResolved {
		return nil, fmt.Errorf("%w: status", ErrInvalidOpsFilter)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		id, dedupe_key, source, resource, title, message, severity, status, occurrences,
		metadata, first_seen_at, last_seen_at, acked_at, resolved_at
	FROM ops_alerts
	WHERE (? = '' OR status = ?)
	ORDER BY last_seen_at DESC, id DESC
	LIMIT ?`, status, status, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]OpsAlert, 0, limit)
	for rows.Next() {
		var item OpsAlert
		if err := rows.Scan(
			&item.ID,
			&item.DedupeKey,
			&item.Source,
			&item.Resource,
			&item.Title,
			&item.Message,
			&item.Severity,
			&item.Status,
			&item.Occurrences,
			&item.Metadata,
			&item.FirstSeenAt,
			&item.LastSeenAt,
			&item.AckedAt,
			&item.ResolvedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) AckOpsAlert(ctx context.Context, id int64, ackAt time.Time) (OpsAlert, error) {
	if id <= 0 {
		return OpsAlert{}, sql.ErrNoRows
	}
	at := ackAt.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	atRFC3339 := at.Format(time.RFC3339)

	result, err := s.db.ExecContext(ctx, `UPDATE ops_alerts
		SET status = ?,
		    acked_at = ?,
		    last_seen_at = ?
		WHERE id = ?
		  AND status != ?`,
		opsAlertStatusAcked,
		atRFC3339,
		atRFC3339,
		id,
		opsAlertStatusResolved,
	)
	if err != nil {
		return OpsAlert{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return OpsAlert{}, err
	}
	if affected == 0 {
		return OpsAlert{}, sql.ErrNoRows
	}

	var out OpsAlert
	err = s.db.QueryRowContext(ctx, `SELECT
		id, dedupe_key, source, resource, title, message, severity, status, occurrences,
		metadata, first_seen_at, last_seen_at, acked_at, resolved_at
	FROM ops_alerts
	WHERE id = ?`, id).Scan(
		&out.ID,
		&out.DedupeKey,
		&out.Source,
		&out.Resource,
		&out.Title,
		&out.Message,
		&out.Severity,
		&out.Status,
		&out.Occurrences,
		&out.Metadata,
		&out.FirstSeenAt,
		&out.LastSeenAt,
		&out.AckedAt,
		&out.ResolvedAt,
	)
	if err != nil {
		return OpsAlert{}, err
	}
	return out, nil
}

func (s *Store) ListOpsRunbooks(ctx context.Context) ([]OpsRunbook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, name, description, steps_json, enabled, created_at, updated_at
	FROM ops_runbooks
	ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runbooks := make([]OpsRunbook, 0, 8)
	for rows.Next() {
		var (
			item      OpsRunbook
			stepsJSON string
			enabled   int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&stepsJSON,
			&enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(stepsJSON), &item.Steps); err != nil {
			item.Steps = []OpsRunbookStep{}
		}
		runbooks = append(runbooks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runbooks, nil
}

func (s *Store) StartOpsRunbook(ctx context.Context, runbookID string, at time.Time) (OpsRunbookRun, error) {
	runbookID = strings.TrimSpace(runbookID)
	if runbookID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}

	runbook, err := s.getOpsRunbookByID(ctx, runbookID)
	if err != nil {
		return OpsRunbookRun{}, err
	}

	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := uuid.NewString()
	totalSteps := len(runbook.Steps)
	currentStep := ""
	if totalSteps > 0 {
		currentStep = runbook.Steps[0].Title
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, '', '')`,
		runID,
		runbook.ID,
		runbook.Name,
		opsRunbookStatusQueued,
		totalSteps,
		0,
		currentStep,
		now.Format(time.RFC3339),
	); err != nil {
		return OpsRunbookRun{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE ops_runbook_runs
		SET status = ?, started_at = ?, current_step = ?
		WHERE id = ?`,
		opsRunbookStatusRunning,
		now.Format(time.RFC3339),
		currentStep,
		runID,
	); err != nil {
		return OpsRunbookRun{}, err
	}

	finalStep := "completed"
	if totalSteps > 0 {
		finalStep = runbook.Steps[totalSteps-1].Title
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ops_runbook_runs
		SET status = ?, completed_steps = ?, current_step = ?, finished_at = ?
		WHERE id = ?`,
		opsRunbookStatusSucceeded,
		totalSteps,
		finalStep,
		now.Format(time.RFC3339),
		runID,
	); err != nil {
		return OpsRunbookRun{}, err
	}

	if err := tx.Commit(); err != nil {
		return OpsRunbookRun{}, err
	}

	return s.GetOpsRunbookRun(ctx, runID)
}

func (s *Store) ListOpsRunbookRuns(ctx context.Context, limit int) ([]OpsRunbookRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
	FROM ops_runbook_runs
	ORDER BY created_at DESC, id DESC
	LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]OpsRunbookRun, 0, limit)
	for rows.Next() {
		item, err := scanOpsRunbookRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetOpsRunbookRun(ctx context.Context, runID string) (OpsRunbookRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
	FROM ops_runbook_runs
	WHERE id = ?
	LIMIT 1`, runID)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	item, err := scanOpsRunbookRun(rows)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	if err := rows.Err(); err != nil {
		return OpsRunbookRun{}, err
	}
	return item, nil
}

func (s *Store) getOpsRunbookByID(ctx context.Context, runbookID string) (OpsRunbook, error) {
	var (
		out      OpsRunbook
		stepsRaw string
		enabled  int
	)
	err := s.db.QueryRowContext(ctx, `SELECT
		id, name, description, steps_json, enabled, created_at, updated_at
	FROM ops_runbooks
	WHERE id = ?`, runbookID).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&stepsRaw,
		&enabled,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return OpsRunbook{}, err
	}
	out.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(stepsRaw), &out.Steps); err != nil {
		out.Steps = []OpsRunbookStep{}
	}
	return out, nil
}

type opsRunbookRunScanner interface {
	Scan(dest ...any) error
}

func scanOpsRunbookRun(scanner opsRunbookRunScanner) (OpsRunbookRun, error) {
	var out OpsRunbookRun
	if err := scanner.Scan(
		&out.ID,
		&out.RunbookID,
		&out.RunbookName,
		&out.Status,
		&out.TotalSteps,
		&out.CompletedSteps,
		&out.CurrentStep,
		&out.Error,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	); err != nil {
		return OpsRunbookRun{}, err
	}
	return out, nil
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
