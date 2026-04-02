package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	TmuxLauncherCwdModeSession    = "session"
	TmuxLauncherCwdModeActivePane = "active-pane"
	TmuxLauncherCwdModeFixed      = "fixed"
)

type TmuxLauncher struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Icon       string    `json:"icon"`
	Command    string    `json:"command"`
	CwdMode    string    `json:"cwdMode"`
	CwdValue   string    `json:"cwdValue"`
	WindowName string    `json:"windowName"`
	UserMode   string    `json:"userMode"`
	UserValue  string    `json:"userValue"`
	SortOrder  int       `json:"sortOrder"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

type TmuxLauncherWrite struct {
	Name       string
	Icon       string
	Command    string
	CwdMode    string
	CwdValue   string
	WindowName string
	UserMode   string
	UserValue  string
}

func (s *Store) ListTmuxLaunchers(ctx context.Context) ([]TmuxLauncher, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, icon, command, cwd_mode, cwd_value, window_name, user_mode, user_value, sort_order, created_at, updated_at, last_used_at
		   FROM tmux_launchers
		  ORDER BY sort_order ASC, name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]TmuxLauncher, 0, 8)
	for rows.Next() {
		var (
			row                                TmuxLauncher
			createdAtRaw, updatedAtRaw, usedAt string
		)
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Icon,
			&row.Command,
			&row.CwdMode,
			&row.CwdValue,
			&row.WindowName,
			&row.UserMode,
			&row.UserValue,
			&row.SortOrder,
			&createdAtRaw,
			&updatedAtRaw,
			&usedAt,
		); err != nil {
			return nil, err
		}
		row.CreatedAt = parseStoreTime(createdAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.LastUsedAt = parseStoreTime(usedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) GetTmuxLauncher(ctx context.Context, id string) (TmuxLauncher, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TmuxLauncher{}, errors.New("tmux launcher id is required")
	}

	var (
		row                                TmuxLauncher
		createdAtRaw, updatedAtRaw, usedAt string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, icon, command, cwd_mode, cwd_value, window_name, user_mode, user_value, sort_order, created_at, updated_at, last_used_at
		   FROM tmux_launchers
		  WHERE id = ?`,
		id,
	).Scan(
		&row.ID,
		&row.Name,
		&row.Icon,
		&row.Command,
		&row.CwdMode,
		&row.CwdValue,
		&row.WindowName,
		&row.UserMode,
		&row.UserValue,
		&row.SortOrder,
		&createdAtRaw,
		&updatedAtRaw,
		&usedAt,
	)
	if err != nil {
		return TmuxLauncher{}, err
	}
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.UpdatedAt = parseStoreTime(updatedAtRaw)
	row.LastUsedAt = parseStoreTime(usedAt)
	return row, nil
}

func (s *Store) CreateTmuxLauncher(ctx context.Context, row TmuxLauncherWrite) (TmuxLauncher, error) {
	normalized, err := normalizeTmuxLauncherWrite(row)
	if err != nil {
		return TmuxLauncher{}, err
	}

	id := randomID()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tmux_launchers (
		   id, name, icon, command, cwd_mode, cwd_value, window_name, user_mode, user_value, sort_order, created_at, updated_at, last_used_at
		 )
		 VALUES (
		   ?, ?, ?, ?, ?, ?, ?, ?, ?,
		   COALESCE((SELECT MAX(sort_order) + 1 FROM tmux_launchers), 1),
		   datetime('now'),
		   datetime('now'),
		   ''
		 )`,
		id,
		normalized.Name,
		normalized.Icon,
		normalized.Command,
		normalized.CwdMode,
		normalized.CwdValue,
		normalized.WindowName,
		normalized.UserMode,
		normalized.UserValue,
	); err != nil {
		return TmuxLauncher{}, err
	}

	return s.GetTmuxLauncher(ctx, id)
}

func (s *Store) UpdateTmuxLauncher(ctx context.Context, id string, row TmuxLauncherWrite) (TmuxLauncher, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TmuxLauncher{}, errors.New("tmux launcher id is required")
	}

	normalized, err := normalizeTmuxLauncherWrite(row)
	if err != nil {
		return TmuxLauncher{}, err
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE tmux_launchers
		    SET name = ?, icon = ?, command = ?, cwd_mode = ?, cwd_value = ?, window_name = ?, user_mode = ?, user_value = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		normalized.Name,
		normalized.Icon,
		normalized.Command,
		normalized.CwdMode,
		normalized.CwdValue,
		normalized.WindowName,
		normalized.UserMode,
		normalized.UserValue,
		id,
	)
	if err != nil {
		return TmuxLauncher{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return TmuxLauncher{}, err
	}
	if affected == 0 {
		return TmuxLauncher{}, sql.ErrNoRows
	}

	return s.GetTmuxLauncher(ctx, id)
}

func (s *Store) DeleteTmuxLauncher(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tmux launcher id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM tmux_launchers WHERE id = ?`, id)
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

func (s *Store) ReorderTmuxLaunchers(ctx context.Context, ids []string) error {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, item := range ids {
		id := strings.TrimSpace(item)
		if id == "" {
			return errors.New("tmux launcher id is required")
		}
		if _, exists := seen[id]; exists {
			return errors.New("duplicate tmux launcher id")
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for index, id := range normalized {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tmux_launchers
			    SET sort_order = ?, updated_at = datetime('now')
			  WHERE id = ?`,
			index+1, id,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) MarkTmuxLauncherUsed(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tmux launcher id is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE tmux_launchers
		    SET last_used_at = datetime('now'),
		        updated_at = datetime('now')
		  WHERE id = ?`,
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

const (
	TmuxLauncherUserModeSession = "session"
	TmuxLauncherUserModeFixed   = "fixed"
)

func normalizeTmuxLauncherWrite(row TmuxLauncherWrite) (TmuxLauncherWrite, error) {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		return TmuxLauncherWrite{}, errors.New("tmux launcher name is required")
	}
	icon := strings.TrimSpace(row.Icon)
	if icon == "" {
		return TmuxLauncherWrite{}, errors.New("tmux launcher icon is required")
	}
	command := strings.TrimSpace(row.Command)
	cwdMode := strings.TrimSpace(row.CwdMode)
	switch cwdMode {
	case TmuxLauncherCwdModeSession, TmuxLauncherCwdModeActivePane:
		row.CwdValue = ""
	case TmuxLauncherCwdModeFixed:
		row.CwdValue = strings.TrimSpace(row.CwdValue)
		if row.CwdValue == "" {
			return TmuxLauncherWrite{}, errors.New("tmux launcher fixed cwd is required")
		}
	default:
		return TmuxLauncherWrite{}, errors.New("invalid tmux launcher cwd mode")
	}

	windowName := strings.TrimSpace(row.WindowName)
	if windowName == "" {
		windowName = name
	}

	userMode := strings.TrimSpace(row.UserMode)
	userValue := strings.TrimSpace(row.UserValue)
	switch userMode {
	case "", TmuxLauncherUserModeSession:
		userMode = TmuxLauncherUserModeSession
		userValue = ""
	case TmuxLauncherUserModeFixed:
		// userValue kept as-is (can be validated at the API layer)
	default:
		return TmuxLauncherWrite{}, errors.New("invalid tmux launcher user mode")
	}

	return TmuxLauncherWrite{
		Name:       name,
		Icon:       icon,
		Command:    command,
		CwdMode:    cwdMode,
		CwdValue:   row.CwdValue,
		WindowName: windowName,
		UserMode:   userMode,
		UserValue:  userValue,
	}, nil
}
