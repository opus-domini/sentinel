package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// BuildWatchtowerPanePatches returns projection rows suitable for
// client-side pane strip reconciliation without additional API reads.
func BuildWatchtowerPanePatches(panes []WatchtowerPane) []map[string]any {
	patches := make([]map[string]any, 0, len(panes))
	for _, row := range panes {
		changedAt := ""
		if !row.ChangedAt.IsZero() {
			changedAt = row.ChangedAt.UTC().Format(time.RFC3339)
		}
		patches = append(patches, map[string]any{
			wtKeySession:     row.SessionName,
			"windowIndex":    row.WindowIndex,
			"paneIndex":      row.PaneIndex,
			"paneId":         row.PaneID,
			"title":          row.Title,
			"active":         row.Active,
			"tty":            row.TTY,
			"currentPath":    row.CurrentPath,
			"startCommand":   row.StartCommand,
			"currentCommand": row.CurrentCommand,
			"tailPreview":    row.TailPreview,
			"revision":       row.Revision,
			"seenRevision":   row.SeenRevision,
			"hasUnread":      row.Revision > row.SeenRevision,
			"changedAt":      changedAt,
		})
	}
	return patches
}

// UpsertWatchtowerPane upserts watchtower pane.
func (s *Store) UpsertWatchtowerPane(ctx context.Context, row WatchtowerPaneWrite) error {
	paneID := strings.TrimSpace(row.PaneID)
	if paneID == "" {
		return errors.New("pane id is required")
	}
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_panes (
			pane_id, session_name, window_index, pane_index, title,
			active, tty, current_path, start_command, current_command,
			tail_hash, tail_preview, tail_captured_at,
			revision, seen_revision, changed_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pane_id) DO UPDATE SET
			session_name = excluded.session_name,
			window_index = excluded.window_index,
			pane_index = excluded.pane_index,
			title = excluded.title,
			active = excluded.active,
			tty = excluded.tty,
			current_path = excluded.current_path,
			start_command = excluded.start_command,
			current_command = excluded.current_command,
			tail_hash = excluded.tail_hash,
			tail_preview = excluded.tail_preview,
			tail_captured_at = excluded.tail_captured_at,
			revision = excluded.revision,
			seen_revision = excluded.seen_revision,
			changed_at = excluded.changed_at,
			updated_at = excluded.updated_at`,
		paneID,
		name,
		row.WindowIndex,
		row.PaneIndex,
		strings.TrimSpace(row.Title),
		boolToInt(row.Active),
		strings.TrimSpace(row.TTY),
		strings.TrimSpace(row.CurrentPath),
		strings.TrimSpace(row.StartCommand),
		strings.TrimSpace(row.CurrentCommand),
		strings.TrimSpace(row.TailHash),
		strings.TrimSpace(row.TailPreview),
		formatStoreValueTime(row.TailCapturedAt),
		row.Revision,
		row.SeenRevision,
		formatStoreValueTime(row.ChangedAt),
		updatedAt.Format(time.RFC3339),
	)
	return err
}

// ListWatchtowerPanes lists watchtower panes.
func (s *Store) ListWatchtowerPanes(ctx context.Context, sessionName string) ([]WatchtowerPane, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT pane_id, session_name, window_index, pane_index, title,
		        active, tty, current_path, start_command, current_command,
		        tail_hash, tail_preview, tail_captured_at,
		        revision, seen_revision, changed_at, updated_at
		   FROM wt_panes
		  WHERE session_name = ?
		  ORDER BY window_index ASC, pane_index ASC`,
		strings.TrimSpace(sessionName),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPane, 0, 16)
	for rows.Next() {
		var (
			row                                   WatchtowerPane
			activeRaw                             int
			tailCapturedRaw, changedAt, updatedAt string
		)
		if err := rows.Scan(
			&row.PaneID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneIndex,
			&row.Title,
			&activeRaw,
			&row.TTY,
			&row.CurrentPath,
			&row.StartCommand,
			&row.CurrentCommand,
			&row.TailHash,
			&row.TailPreview,
			&tailCapturedRaw,
			&row.Revision,
			&row.SeenRevision,
			&changedAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		row.Active = activeRaw == 1
		row.TailCapturedAt = parseStoreTime(tailCapturedRaw)
		row.ChangedAt = parseStoreTime(changedAt)
		row.UpdatedAt = parseStoreTime(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

// PurgeWatchtowerPanes purges watchtower panes.
func (s *Store) PurgeWatchtowerPanes(ctx context.Context, sessionName string, activePaneIDs []string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	if len(activePaneIDs) == 0 {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM wt_panes WHERE session_name = ?",
			sessionName,
		)
		return err
	}

	placeholders := sqlPlaceholders(len(activePaneIDs))
	args := make([]any, 0, len(activePaneIDs)+1)
	args = append(args, sessionName)
	args = append(args, stringsToAny(activePaneIDs)...)
	query := "DELETE FROM wt_panes WHERE session_name = ? AND pane_id NOT IN (" + placeholders + ")" //nolint:gosec // placeholders are generated literals
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// MarkWatchtowerPaneSeen marks watchtower pane seen.
func (s *Store) MarkWatchtowerPaneSeen(ctx context.Context, sessionName, paneID string) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	paneID = strings.TrimSpace(paneID)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	if paneID == "" {
		return false, errors.New("pane id is required")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND pane_id = ?
		    AND revision > seen_revision`,
		sessionName,
		paneID,
	)
}

// MarkWatchtowerWindowSeen marks watchtower window seen.
func (s *Store) MarkWatchtowerWindowSeen(ctx context.Context, sessionName string, windowIndex int) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	if windowIndex < 0 {
		return false, errors.New("window index must be >= 0")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND window_index = ?
		    AND revision > seen_revision`,
		sessionName,
		windowIndex,
	)
}

// MarkWatchtowerSessionSeen marks watchtower session seen.
func (s *Store) MarkWatchtowerSessionSeen(ctx context.Context, sessionName string) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND revision > seen_revision`,
		sessionName,
	)
}

func (s *Store) markWatchtowerSeen(ctx context.Context, sessionName, updateQuery string, args ...any) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return false, err
	}
	if err := recomputeWatchtowerUnreadTx(ctx, tx, sessionName); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func recomputeWatchtowerUnreadTx(ctx context.Context, tx *sql.Tx, sessionName string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil
	}

	windowUnread := make(map[int]int)
	rows, err := tx.QueryContext(ctx,
		`SELECT window_index, COUNT(*)
		   FROM wt_panes
		  WHERE session_name = ?
		    AND revision > seen_revision
		  GROUP BY window_index`,
		sessionName,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var idx, count int
		if err := rows.Scan(&idx, &count); err != nil {
			_ = rows.Close()
			return err
		}
		windowUnread[idx] = count
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	wRows, err := tx.QueryContext(ctx,
		`SELECT window_index, unread_panes, has_unread
		   FROM wt_windows
		  WHERE session_name = ?`,
		sessionName,
	)
	if err != nil {
		return err
	}
	defer func() { _ = wRows.Close() }()

	unreadWindows := 0
	unreadPanes := 0
	for wRows.Next() {
		var idx, prevUnread, prevHasUnread int
		if err := wRows.Scan(&idx, &prevUnread, &prevHasUnread); err != nil {
			return err
		}
		nextUnread := windowUnread[idx]
		nextHasUnread := 0
		if nextUnread > 0 {
			nextHasUnread = 1
			unreadWindows++
			unreadPanes += nextUnread
		}

		if nextUnread != prevUnread || nextHasUnread != prevHasUnread {
			if _, err := tx.ExecContext(ctx,
				`UPDATE wt_windows
				    SET unread_panes = ?,
				        has_unread = ?,
				        rev = rev + 1,
				        updated_at = datetime('now')
				  WHERE session_name = ?
				    AND window_index = ?`,
				nextUnread,
				nextHasUnread,
				sessionName,
				idx,
			); err != nil {
				return err
			}
		}
	}
	if err := wRows.Err(); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE wt_sessions
		    SET unread_panes = ?,
		        unread_windows = ?,
		        rev = CASE
		              WHEN unread_panes <> ? OR unread_windows <> ? THEN rev + 1
		              ELSE rev
		            END,
		        updated_at = datetime('now')
		  WHERE session_name = ?`,
		unreadPanes,
		unreadWindows,
		unreadPanes,
		unreadWindows,
		sessionName,
	); err != nil {
		return err
	}
	return nil
}
