package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// SessionLauncher represents session launcher data.
type SessionLauncher struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Cwd        string    `json:"cwd"`
	Icon       string    `json:"icon"`
	User       string    `json:"user"`
	SortOrder  int       `json:"sortOrder"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	UseCount   int       `json:"useCount"`
}

// SessionLauncherWrite represents session launcher write data.
type SessionLauncherWrite struct {
	Name string
	Cwd  string
	Icon string
	User string
}

// ListSessionLaunchers lists session launchers.
func (s *Store) ListSessionLaunchers(ctx context.Context) ([]SessionLauncher, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, cwd, icon, user, sort_order, created_at, updated_at, last_used_at, use_count
		   FROM session_launchers
		  ORDER BY sort_order ASC, name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionLauncher, 0, 8)
	for rows.Next() {
		var (
			row                        SessionLauncher
			createdAtRaw, updatedAtRaw string
			usedAtRaw                  string
		)
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Cwd,
			&row.Icon,
			&row.User,
			&row.SortOrder,
			&createdAtRaw,
			&updatedAtRaw,
			&usedAtRaw,
			&row.UseCount,
		); err != nil {
			return nil, err
		}
		row.CreatedAt = parseStoreTime(createdAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.LastUsedAt = parseStoreTime(usedAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetSessionLauncher returns session launcher.
func (s *Store) GetSessionLauncher(ctx context.Context, id string) (SessionLauncher, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionLauncher{}, errors.New("session launcher id is required")
	}

	var (
		row                        SessionLauncher
		createdAtRaw, updatedAtRaw string
		usedAtRaw                  string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, cwd, icon, user, sort_order, created_at, updated_at, last_used_at, use_count
		   FROM session_launchers
		  WHERE id = ?`,
		id,
	).Scan(
		&row.ID,
		&row.Name,
		&row.Cwd,
		&row.Icon,
		&row.User,
		&row.SortOrder,
		&createdAtRaw,
		&updatedAtRaw,
		&usedAtRaw,
		&row.UseCount,
	)
	if err != nil {
		return SessionLauncher{}, err
	}
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.UpdatedAt = parseStoreTime(updatedAtRaw)
	row.LastUsedAt = parseStoreTime(usedAtRaw)
	return row, nil
}

// CreateSessionLauncher creates session launcher.
func (s *Store) CreateSessionLauncher(ctx context.Context, row SessionLauncherWrite) (SessionLauncher, error) {
	normalized, err := normalizeSessionLauncherWrite(row)
	if err != nil {
		return SessionLauncher{}, err
	}

	id := randomID()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO session_launchers (
		   id, name, cwd, icon, user, sort_order, created_at, updated_at, last_used_at, use_count
		 )
		 VALUES (
		   ?, ?, ?, ?, ?,
		   COALESCE((SELECT MAX(sort_order) + 1 FROM session_launchers), 1),
		   datetime('now'),
		   datetime('now'),
		   '',
		   0
		 )`,
		id,
		normalized.Name,
		normalized.Cwd,
		normalized.Icon,
		normalized.User,
	); err != nil {
		return SessionLauncher{}, err
	}

	return s.GetSessionLauncher(ctx, id)
}

// UpdateSessionLauncher updates session launcher.
func (s *Store) UpdateSessionLauncher(ctx context.Context, id string, row SessionLauncherWrite) (SessionLauncher, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionLauncher{}, errors.New("session launcher id is required")
	}

	normalized, err := normalizeSessionLauncherWrite(row)
	if err != nil {
		return SessionLauncher{}, err
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE session_launchers
		    SET name = ?, cwd = ?, icon = ?, user = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		normalized.Name,
		normalized.Cwd,
		normalized.Icon,
		normalized.User,
		id,
	)
	if err != nil {
		return SessionLauncher{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return SessionLauncher{}, err
	}
	if affected == 0 {
		return SessionLauncher{}, sql.ErrNoRows
	}

	return s.GetSessionLauncher(ctx, id)
}

// DeleteSessionLauncher deletes session launcher.
func (s *Store) DeleteSessionLauncher(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session launcher id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM session_launchers WHERE id = ?`, id)
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

// ReorderSessionLaunchers reorders session launchers.
func (s *Store) ReorderSessionLaunchers(ctx context.Context, ids []string) error {
	normalized, err := normalizeSessionLauncherIDs(ids)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for index, id := range normalized {
		if _, err := tx.ExecContext(ctx,
			`UPDATE session_launchers
			    SET sort_order = ?, updated_at = datetime('now')
			  WHERE id = ?`,
			index+1, id,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// MarkSessionLauncherUsed marks session launcher used.
func (s *Store) MarkSessionLauncherUsed(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session launcher id is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE session_launchers
		    SET last_used_at = datetime('now'),
		        use_count = use_count + 1,
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

func normalizeSessionLauncherWrite(row SessionLauncherWrite) (SessionLauncherWrite, error) {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		return SessionLauncherWrite{}, errors.New("session launcher name is required")
	}
	cwd := strings.TrimSpace(row.Cwd)
	if cwd == "" {
		return SessionLauncherWrite{}, errors.New("session launcher cwd is required")
	}
	icon := strings.TrimSpace(row.Icon)
	if icon == "" {
		return SessionLauncherWrite{}, errors.New("session launcher icon is required")
	}
	return SessionLauncherWrite{
		Name: name,
		Cwd:  cwd,
		Icon: icon,
		User: strings.TrimSpace(row.User),
	}, nil
}

func normalizeSessionLauncherIDs(ids []string) ([]string, error) {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, item := range ids {
		id := strings.TrimSpace(item)
		if id == "" {
			return nil, errors.New("session launcher id is required")
		}
		if _, exists := seen[id]; exists {
			return nil, errors.New("duplicate session launcher id")
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, nil
}
