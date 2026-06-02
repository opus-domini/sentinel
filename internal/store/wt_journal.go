package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// InsertWatchtowerJournal inserts watchtower journal.
func (s *Store) InsertWatchtowerJournal(ctx context.Context, row WatchtowerJournalWrite) (int64, error) {
	entityType := strings.TrimSpace(row.EntityType)
	if entityType == "" {
		return 0, errors.New("entity type is required")
	}
	changedAt := row.ChangedAt.UTC()
	if changedAt.IsZero() {
		changedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_journal (
			global_rev, entity_type, session_name, window_index,
			pane_id, change_kind, changed_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.GlobalRev,
		entityType,
		strings.TrimSpace(row.Session),
		row.WindowIdx,
		strings.TrimSpace(row.PaneID),
		strings.TrimSpace(row.ChangeKind),
		changedAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ListWatchtowerJournalSince lists watchtower journal since.
func (s *Store) ListWatchtowerJournalSince(ctx context.Context, sinceRev int64, limit int) ([]WatchtowerJournal, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, global_rev, entity_type, session_name, window_index,
		        pane_id, change_kind, changed_at
		   FROM wt_journal
		  WHERE global_rev > ?
		  ORDER BY global_rev ASC, id ASC
		  LIMIT ?`,
		sinceRev,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerJournal, 0, limit)
	for rows.Next() {
		var (
			row          WatchtowerJournal
			changedAtRaw string
		)
		if err := rows.Scan(
			&row.ID,
			&row.GlobalRev,
			&row.EntityType,
			&row.Session,
			&row.WindowIdx,
			&row.PaneID,
			&row.ChangeKind,
			&changedAtRaw,
		); err != nil {
			return nil, err
		}
		row.ChangedAt = parseStoreTime(changedAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

// PruneWatchtowerJournalRows prunes watchtower journal rows.
func (s *Store) PruneWatchtowerJournalRows(ctx context.Context, maxRows int) (int64, error) {
	if maxRows <= 0 {
		return 0, nil
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wt_journal
		  WHERE id IN (
			SELECT id
			  FROM wt_journal
			 ORDER BY global_rev DESC, id DESC
			 LIMIT -1 OFFSET ?
		  )`,
		maxRows,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SetWatchtowerRuntimeValue sets watchtower runtime value.
func (s *Store) SetWatchtowerRuntimeValue(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_runtime (key, value, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(key),
		value,
	)
	return err
}

// SetWatchtowerRuntimeValues upserts several runtime key/values in a single
// transaction (one fsync) instead of one auto-committed write per key, cutting
// the per-tick metric write amplification on the single SQLite connection.
func (s *Store) SetWatchtowerRuntimeValues(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO wt_runtime (key, value, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for key, value := range values {
		if _, err := stmt.ExecContext(ctx, strings.TrimSpace(key), value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetWatchtowerRuntimeValue returns watchtower runtime value.
func (s *Store) GetWatchtowerRuntimeValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM wt_runtime WHERE key = ?`,
		strings.TrimSpace(key),
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// WatchtowerGlobalRevision returns the current global revision counter,
// or 0 if the value is absent or unparseable.
func (s *Store) WatchtowerGlobalRevision(ctx context.Context) (int64, error) {
	raw, err := s.GetWatchtowerRuntimeValue(ctx, "global_rev")
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
