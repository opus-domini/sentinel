package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
)

func (s *Store) UpsertAlert(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error) {
	now := write.CreatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dedupeKey := strings.TrimSpace(write.DedupeKey)
	if dedupeKey == "" {
		return alerts.Alert{}, fmt.Errorf("dedupe key is required")
	}
	source := strings.TrimSpace(write.Source)
	if source == "" {
		source = activity.DefaultSource
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
	severity := activity.NormalizeSeverity(write.Severity)
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
		alerts.StatusOpen,
		metadata,
		nowRFC3339,
		nowRFC3339,
		alerts.StatusResolved,
		alerts.StatusOpen,
	); err != nil {
		return alerts.Alert{}, err
	}

	return s.getAlertByDedupeKey(ctx, dedupeKey)
}

func (s *Store) getAlertByDedupeKey(ctx context.Context, dedupeKey string) (alerts.Alert, error) {
	var out alerts.Alert
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
		return alerts.Alert{}, err
	}
	return out, nil
}

func (s *Store) ListAlerts(ctx context.Context, limit int, status string) ([]alerts.Alert, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != alerts.StatusOpen && status != alerts.StatusAcked && status != alerts.StatusResolved {
		return nil, fmt.Errorf("%w: status", alerts.ErrInvalidFilter)
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

	items := make([]alerts.Alert, 0, limit)
	for rows.Next() {
		var item alerts.Alert
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

func (s *Store) AckAlert(ctx context.Context, id int64, ackAt time.Time) (alerts.Alert, error) {
	if id <= 0 {
		return alerts.Alert{}, sql.ErrNoRows
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
		alerts.StatusAcked,
		atRFC3339,
		atRFC3339,
		id,
		alerts.StatusResolved,
	)
	if err != nil {
		return alerts.Alert{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return alerts.Alert{}, err
	}
	if affected == 0 {
		return alerts.Alert{}, sql.ErrNoRows
	}

	var out alerts.Alert
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
		return alerts.Alert{}, err
	}
	return out, nil
}

func (s *Store) DeleteAlert(ctx context.Context, id int64) error {
	if id <= 0 {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM ops_alerts WHERE id = ? AND status = ?`,
		id, alerts.StatusResolved,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error) {
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey == "" {
		return alerts.Alert{}, sql.ErrNoRows
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE ops_alerts
		SET status = ?, resolved_at = ?, last_seen_at = ?
		WHERE dedupe_key = ? AND status != ?`,
		alerts.StatusResolved, nowRFC3339, nowRFC3339, dedupeKey, alerts.StatusResolved,
	)
	if err != nil {
		return alerts.Alert{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return alerts.Alert{}, err
	}
	if affected == 0 {
		return alerts.Alert{}, sql.ErrNoRows
	}
	return s.getAlertByDedupeKey(ctx, dedupeKey)
}
