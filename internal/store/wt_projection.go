package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// sqlExecer is satisfied by both *sql.DB and *sql.Tx, so a watchtower write can
// run standalone or as one statement inside a larger transaction.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// WatchtowerSessionProjection is everything one collector tick observed about a
// single tmux session.
type WatchtowerSessionProjection struct {
	Session             WatchtowerSessionWrite
	Windows             []WatchtowerWindowWrite
	Panes               []WatchtowerPaneWrite
	ActivePaneIDs       []string
	ActiveWindowIndices []int
}

// WriteWatchtowerSessionProjection commits one session's panes, windows and
// session row — plus the two purges that drop what tmux no longer reports — in
// a single transaction. The collector used to issue one auto-committed write
// per pane, per window and per purge, so a host with a handful of sessions paid
// dozens of transactions per second on the single SQLite connection that every
// HTTP handler also queues behind. Grouping also makes a tick all-or-nothing:
// a failure halfway through no longer leaves the projection half-updated.
func (s *Store) WriteWatchtowerSessionProjection(ctx context.Context, projection WatchtowerSessionProjection) error {
	sessionName := strings.TrimSpace(projection.Session.SessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, pane := range projection.Panes {
		if err := upsertWatchtowerPane(ctx, tx, pane); err != nil {
			return err
		}
	}
	if err := purgeWatchtowerPanes(ctx, tx, sessionName, projection.ActivePaneIDs); err != nil {
		return err
	}
	for _, window := range projection.Windows {
		if err := upsertWatchtowerWindow(ctx, tx, window); err != nil {
			return err
		}
	}
	if err := purgeWatchtowerWindows(ctx, tx, sessionName, projection.ActiveWindowIndices); err != nil {
		return err
	}
	if err := upsertWatchtowerSession(ctx, tx, projection.Session); err != nil {
		return err
	}
	return tx.Commit()
}
