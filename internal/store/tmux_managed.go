package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type ManagedTmuxWindow struct {
	ID              string    `json:"id"`
	SessionName     string    `json:"sessionName"`
	LauncherID      string    `json:"launcherId"`
	LauncherName    string    `json:"launcherName"`
	Icon            string    `json:"icon"`
	Command         string    `json:"command"`
	CwdMode         string    `json:"cwdMode"`
	CwdValue        string    `json:"cwdValue"`
	ResolvedCwd     string    `json:"resolvedCwd"`
	WindowName      string    `json:"windowName"`
	TmuxWindowID    string    `json:"tmuxWindowId"`
	LastWindowIndex int       `json:"lastWindowIndex"`
	SortOrder       int       `json:"sortOrder"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ManagedTmuxWindowWrite struct {
	SessionName     string
	LauncherID      string
	LauncherName    string
	Icon            string
	Command         string
	CwdMode         string
	CwdValue        string
	ResolvedCwd     string
	WindowName      string
	TmuxWindowID    string
	LastWindowIndex int
}

func (s *Store) ListManagedTmuxWindowsBySession(ctx context.Context, sessionName string) ([]ManagedTmuxWindow, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil, errors.New("session name is required")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_name, launcher_id, launcher_name, icon, command, cwd_mode, cwd_value,
		        resolved_cwd, window_name, tmux_window_id, last_window_index, sort_order, created_at, updated_at
		   FROM managed_tmux_windows
		  WHERE session_name = ?
		  ORDER BY sort_order ASC, created_at ASC, id ASC`,
		sessionName,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]ManagedTmuxWindow, 0, 8)
	for rows.Next() {
		row, err := scanManagedTmuxWindow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) CreateManagedTmuxWindow(ctx context.Context, row ManagedTmuxWindowWrite) (ManagedTmuxWindow, error) {
	normalized, err := normalizeManagedTmuxWindowWrite(row)
	if err != nil {
		return ManagedTmuxWindow{}, err
	}

	id := randomID()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO managed_tmux_windows (
			id, session_name, launcher_id, launcher_name, icon, command, cwd_mode, cwd_value,
			resolved_cwd, window_name, tmux_window_id, last_window_index, sort_order, created_at, updated_at
		 )
		 VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			COALESCE((SELECT MAX(sort_order) + 1 FROM managed_tmux_windows WHERE session_name = ?), 1),
			datetime('now'),
			datetime('now')
		 )`,
		id,
		normalized.SessionName,
		normalized.LauncherID,
		normalized.LauncherName,
		normalized.Icon,
		normalized.Command,
		normalized.CwdMode,
		normalized.CwdValue,
		normalized.ResolvedCwd,
		normalized.WindowName,
		normalized.TmuxWindowID,
		normalized.LastWindowIndex,
		normalized.SessionName,
	); err != nil {
		return ManagedTmuxWindow{}, err
	}

	return s.getManagedTmuxWindow(ctx, id)
}

func (s *Store) UpdateManagedTmuxWindowRuntime(ctx context.Context, id, tmuxWindowID string, lastWindowIndex int) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("managed tmux window id is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE managed_tmux_windows
		    SET tmux_window_id = ?, last_window_index = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		strings.TrimSpace(tmuxWindowID),
		lastWindowIndex,
		id,
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

func (s *Store) UpdateManagedTmuxWindowName(ctx context.Context, id, windowName string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("managed tmux window id is required")
	}
	windowName = strings.TrimSpace(windowName)
	if windowName == "" {
		return errors.New("managed tmux window name is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE managed_tmux_windows
		    SET window_name = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		windowName,
		id,
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

func (s *Store) UpdateManagedTmuxWindowSortOrder(ctx context.Context, id string, sortOrder int) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("managed tmux window id is required")
	}
	if sortOrder < 1 {
		return errors.New("managed tmux window sort order must be >= 1")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE managed_tmux_windows
		    SET sort_order = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		sortOrder,
		id,
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

func (s *Store) DeleteManagedTmuxWindow(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("managed tmux window id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM managed_tmux_windows WHERE id = ?`, id)
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

func (s *Store) DeleteManagedTmuxWindowsMissingRuntime(ctx context.Context, sessionName string, liveWindowIDs []string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	liveSet := make(map[string]struct{}, len(liveWindowIDs))
	for _, item := range liveWindowIDs {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		liveSet[item] = struct{}{}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tmux_window_id
		   FROM managed_tmux_windows
		  WHERE session_name = ?
		    AND tmux_window_id != ''`,
		sessionName,
	)
	if err != nil {
		return err
	}

	var staleIDs []string
	for rows.Next() {
		var (
			id           string
			tmuxWindowID string
		)
		if err := rows.Scan(&id, &tmuxWindowID); err != nil {
			return err
		}
		if _, ok := liveSet[tmuxWindowID]; ok {
			continue
		}
		staleIDs = append(staleIDs, id)
	}
	if err := rows.Err(); err != nil {
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, id := range staleIDs {
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM managed_tmux_windows WHERE id = ?`,
			id,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getManagedTmuxWindow(ctx context.Context, id string) (ManagedTmuxWindow, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ManagedTmuxWindow{}, errors.New("managed tmux window id is required")
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_name, launcher_id, launcher_name, icon, command, cwd_mode, cwd_value,
		        resolved_cwd, window_name, tmux_window_id, last_window_index, sort_order, created_at, updated_at
		   FROM managed_tmux_windows
		  WHERE id = ?`,
		id,
	)
	return scanManagedTmuxWindow(row)
}

type managedTmuxWindowScanner interface {
	Scan(dest ...any) error
}

func scanManagedTmuxWindow(scanner managedTmuxWindowScanner) (ManagedTmuxWindow, error) {
	var (
		row          ManagedTmuxWindow
		createdAtRaw string
		updatedAtRaw string
	)
	if err := scanner.Scan(
		&row.ID,
		&row.SessionName,
		&row.LauncherID,
		&row.LauncherName,
		&row.Icon,
		&row.Command,
		&row.CwdMode,
		&row.CwdValue,
		&row.ResolvedCwd,
		&row.WindowName,
		&row.TmuxWindowID,
		&row.LastWindowIndex,
		&row.SortOrder,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return ManagedTmuxWindow{}, err
	}
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.UpdatedAt = parseStoreTime(updatedAtRaw)
	return row, nil
}

func normalizeManagedTmuxWindowWrite(row ManagedTmuxWindowWrite) (ManagedTmuxWindowWrite, error) {
	row.SessionName = strings.TrimSpace(row.SessionName)
	row.LauncherID = strings.TrimSpace(row.LauncherID)
	row.LauncherName = strings.TrimSpace(row.LauncherName)
	row.Icon = strings.TrimSpace(row.Icon)
	row.Command = strings.TrimSpace(row.Command)
	row.CwdMode = strings.TrimSpace(row.CwdMode)
	row.CwdValue = strings.TrimSpace(row.CwdValue)
	row.ResolvedCwd = strings.TrimSpace(row.ResolvedCwd)
	row.WindowName = strings.TrimSpace(row.WindowName)
	row.TmuxWindowID = strings.TrimSpace(row.TmuxWindowID)

	switch {
	case row.SessionName == "":
		return ManagedTmuxWindowWrite{}, errors.New("session name is required")
	case row.Icon == "":
		return ManagedTmuxWindowWrite{}, errors.New("managed tmux window icon is required")
	case row.WindowName == "":
		return ManagedTmuxWindowWrite{}, errors.New("managed tmux window name is required")
	case row.LastWindowIndex < -1:
		return ManagedTmuxWindowWrite{}, errors.New("managed tmux window index must be >= -1")
	}
	return row, nil
}

func managedWindowsByIndex(rows []ManagedTmuxWindow) map[int]ManagedTmuxWindow {
	byIndex := make(map[int]ManagedTmuxWindow, len(rows))
	for _, row := range rows {
		if row.LastWindowIndex < 0 {
			continue
		}
		byIndex[row.LastWindowIndex] = row
	}
	return byIndex
}
