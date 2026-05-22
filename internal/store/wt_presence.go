package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

// UpsertWatchtowerPresence upserts watchtower presence.
func (s *Store) UpsertWatchtowerPresence(ctx context.Context, row WatchtowerPresenceWrite) error {
	terminalID := strings.TrimSpace(row.TerminalID)
	if terminalID == "" {
		return errors.New("terminal id is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	expiresAt := row.ExpiresAt.UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_presence (
			terminal_id, session_name, window_index, pane_id,
			visible, focused, updated_at, expires_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(terminal_id) DO UPDATE SET
			session_name = excluded.session_name,
			window_index = excluded.window_index,
			pane_id = excluded.pane_id,
			visible = excluded.visible,
			focused = excluded.focused,
			updated_at = excluded.updated_at,
			expires_at = excluded.expires_at`,
		terminalID,
		strings.TrimSpace(row.SessionName),
		row.WindowIndex,
		strings.TrimSpace(row.PaneID),
		boolToInt(row.Visible),
		boolToInt(row.Focused),
		updatedAt.Format(time.RFC3339),
		formatStoreValueTime(expiresAt),
	)
	return err
}

// ListWatchtowerPresence lists watchtower presence.
func (s *Store) ListWatchtowerPresence(ctx context.Context) ([]WatchtowerPresence, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT terminal_id, session_name, window_index, pane_id,
		        visible, focused, updated_at, expires_at
		   FROM wt_presence
		  ORDER BY terminal_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPresence, 0, 8)
	for rows.Next() {
		var (
			row                        WatchtowerPresence
			visibleRaw, focusedRaw     int
			updatedAtRaw, expiresAtRaw string
		)
		if err := rows.Scan(
			&row.TerminalID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneID,
			&visibleRaw,
			&focusedRaw,
			&updatedAtRaw,
			&expiresAtRaw,
		); err != nil {
			return nil, err
		}
		row.Visible = visibleRaw == 1
		row.Focused = focusedRaw == 1
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.ExpiresAt = parseStoreTime(expiresAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListWatchtowerPresenceBySession lists watchtower presence by session.
func (s *Store) ListWatchtowerPresenceBySession(ctx context.Context, sessionName string) ([]WatchtowerPresence, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return []WatchtowerPresence{}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT terminal_id, session_name, window_index, pane_id,
		        visible, focused, updated_at, expires_at
		   FROM wt_presence
		  WHERE session_name = ?
		  ORDER BY terminal_id ASC`,
		sessionName,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPresence, 0, 8)
	for rows.Next() {
		var (
			row                        WatchtowerPresence
			visibleRaw, focusedRaw     int
			updatedAtRaw, expiresAtRaw string
		)
		if err := rows.Scan(
			&row.TerminalID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneID,
			&visibleRaw,
			&focusedRaw,
			&updatedAtRaw,
			&expiresAtRaw,
		); err != nil {
			return nil, err
		}
		row.Visible = visibleRaw == 1
		row.Focused = focusedRaw == 1
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.ExpiresAt = parseStoreTime(expiresAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

// PruneWatchtowerPresence prunes watchtower presence.
func (s *Store) PruneWatchtowerPresence(ctx context.Context, now time.Time) (int64, error) {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wt_presence
		  WHERE expires_at != ''
		    AND expires_at < ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
