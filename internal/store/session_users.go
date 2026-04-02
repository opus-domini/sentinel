package store

import (
	"context"
	"database/sql"
)

// SetSessionUser persists the OS user that owns a tmux session.
func (s *Store) SetSessionUser(ctx context.Context, session, user string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_users (session_name, user, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(session_name) DO UPDATE SET user = excluded.user, updated_at = excluded.updated_at`,
		session, user,
	)
	return err
}

// DeleteSessionUser removes the user mapping for a session.
func (s *Store) DeleteSessionUser(ctx context.Context, session string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_users WHERE session_name = ?`, session)
	return err
}

// ListSessionUsers returns all persisted session-to-user mappings.
func (s *Store) ListSessionUsers(ctx context.Context) (map[string]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT session_name, user FROM session_users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only query

	result := make(map[string]string)
	for rows.Next() {
		var session, user string
		if err := rows.Scan(&session, &user); err != nil {
			continue
		}
		result[session] = user
	}
	return result, rows.Err()
}

// RenameSessionUser migrates a session-user mapping to a new name.
func (s *Store) RenameSessionUser(ctx context.Context, oldName, newName string) error {
	if s == nil || s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // commit follows

	var user string
	err = tx.QueryRowContext(ctx, `SELECT user FROM session_users WHERE session_name = ?`, oldName).Scan(&user)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, _ = tx.ExecContext(ctx, `DELETE FROM session_users WHERE session_name = ?`, oldName)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO session_users (session_name, user, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(session_name) DO UPDATE SET user = excluded.user, updated_at = excluded.updated_at`,
		newName, user,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}
