package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// BuildWatchtowerSessionActivityPatch returns the compact projection pushed to
// clients for session-list reconciliation without requiring full list reloads.
func BuildWatchtowerSessionActivityPatch(row WatchtowerSession) map[string]any {
	activityAt := ""
	if !row.ActivityAt.IsZero() {
		activityAt = row.ActivityAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"name":          row.SessionName,
		"attached":      row.Attached,
		"windows":       row.Windows,
		"panes":         row.Panes,
		"activityAt":    activityAt,
		"lastContent":   row.LastPreview,
		"unreadWindows": row.UnreadWindows,
		"unreadPanes":   row.UnreadPanes,
		"rev":           row.Rev,
	}
}

// BuildWatchtowerInspectorPatch returns a full window/pane projection patch for
// one session, used by ws activity/seen events to avoid inspector polling.
func BuildWatchtowerInspectorPatch(sessionName string, windows []WatchtowerWindow, panes []WatchtowerPane) map[string]any {
	return BuildWatchtowerInspectorPatchWithManaged(sessionName, windows, panes, nil)
}

func BuildWatchtowerInspectorPatchWithManaged(
	sessionName string,
	windows []WatchtowerWindow,
	panes []WatchtowerPane,
	managedByIndex map[int]ManagedTmuxWindow,
) map[string]any {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		if len(windows) > 0 {
			sessionName = strings.TrimSpace(windows[0].SessionName)
		} else if len(panes) > 0 {
			sessionName = strings.TrimSpace(panes[0].SessionName)
		}
	}
	return map[string]any{
		"session": sessionName,
		"windows": BuildWatchtowerWindowPatchesWithManaged(windows, panes, managedByIndex),
		"panes":   BuildWatchtowerPanePatches(panes),
	}
}

func (s *Store) UpsertWatchtowerSession(ctx context.Context, row WatchtowerSessionWrite) error {
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_sessions (
			session_name, attached, windows, panes, activity_at,
			last_preview, last_preview_at, last_preview_pane_id,
			unread_windows, unread_panes, rev, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_name) DO UPDATE SET
			attached = excluded.attached,
			windows = excluded.windows,
			panes = excluded.panes,
			activity_at = excluded.activity_at,
			last_preview = excluded.last_preview,
			last_preview_at = excluded.last_preview_at,
			last_preview_pane_id = excluded.last_preview_pane_id,
			unread_windows = excluded.unread_windows,
			unread_panes = excluded.unread_panes,
			rev = excluded.rev,
			updated_at = excluded.updated_at`,
		name,
		row.Attached,
		row.Windows,
		row.Panes,
		formatStoreValueTime(row.ActivityAt),
		strings.TrimSpace(row.LastPreview),
		formatStoreValueTime(row.LastPreviewAt),
		strings.TrimSpace(row.LastPreviewPaneID),
		row.UnreadWindows,
		row.UnreadPanes,
		row.Rev,
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetWatchtowerSession(ctx context.Context, sessionName string) (WatchtowerSession, error) {
	var (
		row                                     WatchtowerSession
		activityAtRaw, previewAtRaw, updatedRaw string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT session_name, attached, windows, panes, activity_at,
		        last_preview, last_preview_at, last_preview_pane_id,
		        unread_windows, unread_panes, rev, updated_at
		   FROM wt_sessions
		  WHERE session_name = ?`,
		strings.TrimSpace(sessionName),
	).Scan(
		&row.SessionName,
		&row.Attached,
		&row.Windows,
		&row.Panes,
		&activityAtRaw,
		&row.LastPreview,
		&previewAtRaw,
		&row.LastPreviewPaneID,
		&row.UnreadWindows,
		&row.UnreadPanes,
		&row.Rev,
		&updatedRaw,
	)
	if err != nil {
		return WatchtowerSession{}, err
	}
	row.ActivityAt = parseStoreTime(activityAtRaw)
	row.LastPreviewAt = parseStoreTime(previewAtRaw)
	row.UpdatedAt = parseStoreTime(updatedRaw)
	return row, nil
}

func (s *Store) GetWatchtowerSessionActivityPatch(ctx context.Context, sessionName string) (map[string]any, error) {
	row, err := s.GetWatchtowerSession(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	return BuildWatchtowerSessionActivityPatch(row), nil
}

func (s *Store) GetWatchtowerInspectorPatch(ctx context.Context, sessionName string) (map[string]any, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil, errors.New("session name is required")
	}
	windows, err := s.ListWatchtowerWindows(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	panes, err := s.ListWatchtowerPanes(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	managed, err := s.ListManagedTmuxWindowsBySession(ctx, sessionName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return BuildWatchtowerInspectorPatchWithManaged(sessionName, windows, panes, managedWindowsByIndex(managed)), nil
}

func (s *Store) ListWatchtowerSessions(ctx context.Context) ([]WatchtowerSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_name, attached, windows, panes, activity_at,
		        last_preview, last_preview_at, last_preview_pane_id,
		        unread_windows, unread_panes, rev, updated_at
		   FROM wt_sessions
		  ORDER BY session_name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerSession, 0, 16)
	for rows.Next() {
		var (
			row                                     WatchtowerSession
			activityAtRaw, previewAtRaw, updatedRaw string
		)
		if err := rows.Scan(
			&row.SessionName,
			&row.Attached,
			&row.Windows,
			&row.Panes,
			&activityAtRaw,
			&row.LastPreview,
			&previewAtRaw,
			&row.LastPreviewPaneID,
			&row.UnreadWindows,
			&row.UnreadPanes,
			&row.Rev,
			&updatedRaw,
		); err != nil {
			return nil, err
		}
		row.ActivityAt = parseStoreTime(activityAtRaw)
		row.LastPreviewAt = parseStoreTime(previewAtRaw)
		row.UpdatedAt = parseStoreTime(updatedRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PurgeWatchtowerSessions(ctx context.Context, activeSessions []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if len(activeSessions) == 0 {
		for _, stmt := range []string{
			"DELETE FROM wt_panes",
			"DELETE FROM wt_windows",
			"DELETE FROM wt_sessions",
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return tx.Commit()
	}

	placeholders := sqlPlaceholders(len(activeSessions))
	args := stringsToAny(activeSessions)
	for _, item := range []struct {
		table  string
		column string
	}{
		{table: "wt_panes", column: "session_name"},
		{table: "wt_windows", column: "session_name"},
		{table: "wt_sessions", column: "session_name"},
	} {
		query := "DELETE FROM " + item.table + " WHERE " + item.column + " NOT IN (" + placeholders + ")" //nolint:gosec // table/column values are fixed literals
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
