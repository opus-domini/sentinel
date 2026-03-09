package store

import (
	"context"
	"database/sql"
)

// GetOpsRevision returns the current revision for the given table name.
// Returns 0 if the row does not exist.
func (s *Store) GetOpsRevision(ctx context.Context, tableName string) (int64, error) {
	var rev int64
	err := s.db.QueryRowContext(ctx,
		"SELECT rev FROM ops_revisions WHERE table_name = ?",
		tableName,
	).Scan(&rev)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return rev, err
}

// BumpOpsRevision atomically increments the revision for the given table
// and returns the new value. If the row does not exist it is created with
// rev = 1.
func (s *Store) BumpOpsRevision(ctx context.Context, tableName string) (int64, error) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO ops_revisions (table_name, rev) VALUES (?, 1)
		 ON CONFLICT(table_name) DO UPDATE SET rev = rev + 1`,
		tableName,
	); err != nil {
		return 0, err
	}
	return s.GetOpsRevision(ctx, tableName)
}

// Convenience table name constants.
const (
	RevTableAlerts   = "ops_alerts"
	RevTableActivity = "ops_timeline_events"
)

// GetOpsAlertRevision returns the current revision for the alerts table.
func (s *Store) GetOpsAlertRevision(ctx context.Context) (int64, error) {
	return s.GetOpsRevision(ctx, RevTableAlerts)
}

// GetOpsActivityRevision returns the current revision for the activity table.
func (s *Store) GetOpsActivityRevision(ctx context.Context) (int64, error) {
	return s.GetOpsRevision(ctx, RevTableActivity)
}
