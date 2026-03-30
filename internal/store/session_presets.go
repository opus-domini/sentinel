package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type SessionPreset struct {
	Name           string    `json:"name"`
	Cwd            string    `json:"cwd"`
	Icon           string    `json:"icon"`
	SortOrder      int       `json:"sortOrder"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastLaunchedAt time.Time `json:"lastLaunchedAt"`
	LaunchCount    int       `json:"launchCount"`
}

type SessionPresetWrite struct {
	Name string
	Cwd  string
	Icon string
}

func (s *Store) ListSessionPresets(ctx context.Context) ([]SessionPreset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, cwd, icon, sort_order, created_at, updated_at, last_launched_at, launch_count
		   FROM session_presets
		  ORDER BY sort_order ASC, name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionPreset, 0, 8)
	for rows.Next() {
		var (
			row                                     SessionPreset
			createdAtRaw, updatedAtRaw, launchedRaw string
		)
		if err := rows.Scan(
			&row.Name,
			&row.Cwd,
			&row.Icon,
			&row.SortOrder,
			&createdAtRaw,
			&updatedAtRaw,
			&launchedRaw,
			&row.LaunchCount,
		); err != nil {
			return nil, err
		}
		row.CreatedAt = parseStoreTime(createdAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.LastLaunchedAt = parseStoreTime(launchedRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) CreateSessionPreset(ctx context.Context, row SessionPresetWrite) (SessionPreset, error) {
	name, cwd, icon, err := normalizeSessionPresetWrite(row)
	if err != nil {
		return SessionPreset{}, err
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO session_presets (
		   name, cwd, icon, sort_order, created_at, updated_at, last_launched_at, launch_count
		 )
		 VALUES (
		   ?, ?, ?,
		   COALESCE((SELECT MIN(sort_order) - 1 FROM session_presets), 1),
		   datetime('now'),
		   datetime('now'),
		   '',
		   0
		 )`,
		name, cwd, icon,
	); err != nil {
		return SessionPreset{}, err
	}

	return s.getSessionPreset(ctx, name)
}

func (s *Store) UpdateSessionPreset(ctx context.Context, oldName string, row SessionPresetWrite) (SessionPreset, error) {
	oldName = strings.TrimSpace(oldName)
	if oldName == "" {
		return SessionPreset{}, errors.New("session preset name is required")
	}

	name, cwd, icon, err := normalizeSessionPresetWrite(row)
	if err != nil {
		return SessionPreset{}, err
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE session_presets
		    SET name = ?, cwd = ?, icon = ?, updated_at = datetime('now')
		  WHERE name = ?`,
		name, cwd, icon, oldName,
	)
	if err != nil {
		return SessionPreset{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return SessionPreset{}, err
	}
	if affected == 0 {
		return SessionPreset{}, sql.ErrNoRows
	}

	return s.getSessionPreset(ctx, name)
}

func (s *Store) DeleteSessionPreset(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("session preset name is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM session_presets WHERE name = ?`, name)
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

func (s *Store) MarkSessionPresetLaunched(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("session preset name is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE session_presets
		    SET last_launched_at = datetime('now'),
		        launch_count = launch_count + 1,
		        updated_at = datetime('now')
		  WHERE name = ?`,
		name,
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

func (s *Store) ReorderSessionPresets(ctx context.Context, names []string) error {
	normalized, err := normalizeSessionOrderNames(names)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for index, name := range normalized {
		if _, err := tx.ExecContext(ctx,
			`UPDATE session_presets
			    SET sort_order = ?, updated_at = datetime('now')
			  WHERE name = ?`,
			index+1, name,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) getSessionPreset(ctx context.Context, name string) (SessionPreset, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SessionPreset{}, errors.New("session preset name is required")
	}

	var (
		row                                     SessionPreset
		createdAtRaw, updatedAtRaw, launchedRaw string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT name, cwd, icon, sort_order, created_at, updated_at, last_launched_at, launch_count
		   FROM session_presets
		  WHERE name = ?`,
		name,
	).Scan(
		&row.Name,
		&row.Cwd,
		&row.Icon,
		&row.SortOrder,
		&createdAtRaw,
		&updatedAtRaw,
		&launchedRaw,
		&row.LaunchCount,
	)
	if err != nil {
		return SessionPreset{}, err
	}
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.UpdatedAt = parseStoreTime(updatedAtRaw)
	row.LastLaunchedAt = parseStoreTime(launchedRaw)
	return row, nil
}

func normalizeSessionPresetWrite(row SessionPresetWrite) (string, string, string, error) {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		return "", "", "", errors.New("session preset name is required")
	}
	cwd := strings.TrimSpace(row.Cwd)
	if cwd == "" {
		return "", "", "", errors.New("session preset cwd is required")
	}
	icon := strings.TrimSpace(row.Icon)
	if icon == "" {
		return "", "", "", errors.New("session preset icon is required")
	}
	return name, cwd, icon, nil
}
