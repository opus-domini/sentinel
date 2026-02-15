package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type SessionMeta struct {
	Hash        string
	LastContent string
	Icon        string
}

type Store struct {
	db     *sql.DB
	dbPath string
}

func New(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports one concurrent writer. Limit the pool to a
	// single connection so all access is serialized at the Go level,
	// preventing SQLITE_BUSY errors from concurrent HTTP handlers.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set %s: %w", pragma, err)
		}
	}

	schema := `CREATE TABLE IF NOT EXISTS sessions (
		name         TEXT PRIMARY KEY,
		hash         TEXT NOT NULL,
		last_content TEXT DEFAULT '',
		updated_at   TEXT DEFAULT (datetime('now'))
	)`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Migrate: add icon column (idempotent — ignore "duplicate column" error).
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN icon TEXT DEFAULT ''")
	// Migrate: add default naming sequence for tmux windows.
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN next_window_seq INTEGER NOT NULL DEFAULT 1")

	s := &Store{db: db, dbPath: dbPath}
	if err := s.initRecoverySchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create recovery schema: %w", err)
	}
	if err := s.initWatchtowerSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create watchtower schema: %w", err)
	}
	if err := s.initGuardrailSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create guardrail schema: %w", err)
	}

	return s, nil
}

func (s *Store) GetAll(ctx context.Context) (map[string]SessionMeta, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, hash, last_content, icon FROM sessions")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]SessionMeta)
	for rows.Next() {
		var name, hash, content, icon string
		if err := rows.Scan(&name, &hash, &content, &icon); err != nil {
			return nil, err
		}
		result[name] = SessionMeta{Hash: hash, LastContent: content, Icon: icon}
	}
	return result, rows.Err()
}

func (s *Store) UpsertSession(ctx context.Context, name, hash, content string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (name, hash, last_content, updated_at)
		 VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(name) DO UPDATE SET
		   hash = excluded.hash,
		   last_content = excluded.last_content,
		   updated_at = excluded.updated_at`,
		name, hash, content,
	)
	return err
}

func (s *Store) Purge(ctx context.Context, activeNames []string) error {
	if len(activeNames) == 0 {
		_, err := s.db.ExecContext(ctx, "DELETE FROM sessions")
		return err
	}
	placeholders := make([]string, len(activeNames))
	args := make([]any, len(activeNames))
	for i, name := range activeNames {
		placeholders[i] = "?"
		args[i] = name
	}
	query := "DELETE FROM sessions WHERE name NOT IN (" + strings.Join(placeholders, ", ") + ")" //nolint:gosec // placeholders are "?" literals, not user input
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) Rename(ctx context.Context, oldName, newName string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET name = ? WHERE name = ?",
		newName, oldName,
	)
	return err
}

func (s *Store) SetIcon(ctx context.Context, name, icon string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET icon = ?, updated_at = datetime('now') WHERE name = ?",
		icon, name,
	)
	return err
}

func (s *Store) AllocateNextWindowSequence(ctx context.Context, name string, minimum int) (int, error) {
	if minimum < 1 {
		minimum = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sessions (name, hash, last_content, updated_at)
		 VALUES (?, '', '', datetime('now'))
		 ON CONFLICT(name) DO NOTHING`,
		name,
	); err != nil {
		return 0, err
	}

	var current int
	if err := tx.QueryRowContext(ctx,
		"SELECT next_window_seq FROM sessions WHERE name = ?",
		name,
	).Scan(&current); err != nil {
		return 0, err
	}
	if current < minimum {
		current = minimum
	}
	next := current + 1

	if _, err := tx.ExecContext(ctx,
		"UPDATE sessions SET next_window_seq = ? WHERE name = ?",
		next, name,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return current, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
