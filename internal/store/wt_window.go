package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

// BuildWatchtowerWindowPatches returns projection rows suitable for
// client-side window strip reconciliation without additional API reads.
func BuildWatchtowerWindowPatches(windows []WatchtowerWindow, panes []WatchtowerPane) []map[string]any {
	return BuildWatchtowerWindowPatchesWithManaged(windows, panes, nil)
}

func BuildWatchtowerWindowPatchesWithManaged(
	windows []WatchtowerWindow,
	panes []WatchtowerPane,
	managedByIndex map[int]ManagedTmuxWindow,
) []map[string]any {
	paneCounts := make(map[int]int, len(windows))
	for _, pane := range panes {
		paneCounts[pane.WindowIndex]++
	}

	patches := make([]map[string]any, 0, len(windows))
	for _, row := range windows {
		activityAt := ""
		if !row.WindowActivityAt.IsZero() {
			activityAt = row.WindowActivityAt.UTC().Format(time.RFC3339)
		}
		managed := managedByIndex[row.WindowIndex]
		displayName := strings.TrimSpace(row.Name)
		if managedName := strings.TrimSpace(managed.WindowName); managedName != "" {
			displayName = managedName
		}
		patches = append(patches, map[string]any{
			"session":         row.SessionName,
			"tmuxWindowId":    strings.TrimSpace(row.TmuxWindowID),
			"index":           row.WindowIndex,
			"name":            row.Name,
			"displayName":     displayName,
			"displayIcon":     strings.TrimSpace(managed.Icon),
			"managed":         strings.TrimSpace(managed.ID) != "",
			"managedWindowId": strings.TrimSpace(managed.ID),
			"launcherId":      strings.TrimSpace(managed.LauncherID),
			"active":          row.Active,
			"panes":           paneCounts[row.WindowIndex],
			"layout":          row.Layout,
			"unreadPanes":     row.UnreadPanes,
			"hasUnread":       row.HasUnread,
			"rev":             row.Rev,
			"activityAt":      activityAt,
		})
	}
	return patches
}

func (s *Store) UpsertWatchtowerWindow(ctx context.Context, row WatchtowerWindowWrite) error {
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_windows (
			session_name, tmux_window_id, window_index, name, active, layout,
			window_activity_at, unread_panes, has_unread, rev, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_name, window_index) DO UPDATE SET
			tmux_window_id = excluded.tmux_window_id,
			name = excluded.name,
			active = excluded.active,
			layout = excluded.layout,
			window_activity_at = excluded.window_activity_at,
			unread_panes = excluded.unread_panes,
			has_unread = excluded.has_unread,
			rev = excluded.rev,
			updated_at = excluded.updated_at`,
		name,
		strings.TrimSpace(row.TmuxWindowID),
		row.WindowIndex,
		strings.TrimSpace(row.Name),
		boolToInt(row.Active),
		strings.TrimSpace(row.Layout),
		formatStoreValueTime(row.WindowActivityAt),
		row.UnreadPanes,
		boolToInt(row.HasUnread),
		row.Rev,
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListWatchtowerWindows(ctx context.Context, sessionName string) ([]WatchtowerWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_name, tmux_window_id, window_index, name, active, layout,
		        window_activity_at, unread_panes, has_unread, rev, updated_at
		   FROM wt_windows
		  WHERE session_name = ?
		  ORDER BY window_index ASC`,
		strings.TrimSpace(sessionName),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerWindow, 0, 8)
	for rows.Next() {
		var (
			row                            WatchtowerWindow
			activeRaw, unreadRaw           int
			activityAtRaw, updatedAtRawRaw string
		)
		if err := rows.Scan(
			&row.SessionName,
			&row.TmuxWindowID,
			&row.WindowIndex,
			&row.Name,
			&activeRaw,
			&row.Layout,
			&activityAtRaw,
			&row.UnreadPanes,
			&unreadRaw,
			&row.Rev,
			&updatedAtRawRaw,
		); err != nil {
			return nil, err
		}
		row.Active = activeRaw == 1
		row.HasUnread = unreadRaw == 1
		row.WindowActivityAt = parseStoreTime(activityAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRawRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PurgeWatchtowerWindows(ctx context.Context, sessionName string, activeWindowIndices []int) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	if len(activeWindowIndices) == 0 {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM wt_windows WHERE session_name = ?",
			sessionName,
		)
		return err
	}

	placeholders := sqlPlaceholders(len(activeWindowIndices))
	args := make([]any, 0, len(activeWindowIndices)+1)
	args = append(args, sessionName)
	for _, value := range activeWindowIndices {
		args = append(args, value)
	}
	query := "DELETE FROM wt_windows WHERE session_name = ? AND window_index NOT IN (" + placeholders + ")" //nolint:gosec // placeholders are generated literals
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
