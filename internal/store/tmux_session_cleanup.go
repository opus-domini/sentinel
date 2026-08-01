package store

import (
	"context"
	"errors"
	"strings"
)

// DeleteTmuxSessionRuntimeState removes projections tied to one runtime session.
// Durable presets, launchers, journal entries and runbook history are preserved.
func (s *Store) DeleteTmuxSessionRuntimeState(ctx context.Context, sessionName string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("tmux session name is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const sessionMetaNameColumn = "name"
	for _, item := range []struct {
		table  string
		column string
	}{
		{table: "wt_panes", column: wtColSessionName},
		{table: "wt_windows", column: wtColSessionName},
		{table: "wt_sessions", column: wtColSessionName},
		{table: "wt_presence", column: wtColSessionName},
		{table: "managed_tmux_windows", column: wtColSessionName},
		{table: "session_users", column: wtColSessionName},
		{table: "sessions", column: sessionMetaNameColumn},
	} {
		query := "DELETE FROM " + item.table + " WHERE " + item.column + " = ?" //nolint:gosec // fixed table and column literals
		if _, err := tx.ExecContext(ctx, query, sessionName); err != nil {
			return err
		}
	}
	return tx.Commit()
}
