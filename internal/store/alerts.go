package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	opsAlertStatusOpen     = "open"
	opsAlertStatusAcked    = "acked"
	opsAlertStatusResolved = "resolved"
)

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

func (s *Store) initAlertsSchema() error {
	statements := []string{
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
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return err
		}
	}
	return nil
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
		source = opsDefaultSource
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

func (s *Store) ResolveOpsAlert(ctx context.Context, dedupeKey string, at time.Time) (OpsAlert, error) {
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey == "" {
		return OpsAlert{}, sql.ErrNoRows
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE ops_alerts
		SET status = ?, resolved_at = ?, last_seen_at = ?
		WHERE dedupe_key = ? AND status != ?`,
		opsAlertStatusResolved, nowRFC3339, nowRFC3339, dedupeKey, opsAlertStatusResolved,
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
	return s.getOpsAlertByDedupeKey(ctx, dedupeKey)
}
