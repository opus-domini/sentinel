package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// OpsSchedule represents a schedule attached to a runbook.
type OpsSchedule struct {
	ID            string `json:"id"`
	RunbookID     string `json:"runbookId"`
	Name          string `json:"name"`
	ScheduleType  string `json:"scheduleType"` // "cron" or "once"
	CronExpr      string `json:"cronExpr"`     // 5-field cron expression
	Timezone      string `json:"timezone"`     // IANA timezone
	RunAt         string `json:"runAt"`        // ISO8601 for type="once"
	Enabled       bool   `json:"enabled"`
	LastRunAt     string `json:"lastRunAt"`
	LastRunStatus string `json:"lastRunStatus"`
	NextRunAt     string `json:"nextRunAt"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// OpsScheduleWrite is used to create or update a schedule.
type OpsScheduleWrite struct {
	ID           string
	RunbookID    string
	Name         string
	ScheduleType string
	CronExpr     string
	Timezone     string
	RunAt        string
	Enabled      bool
	NextRunAt    string
}

func (s *Store) initSchedulerSchema() error {
	schema := `CREATE TABLE IF NOT EXISTS ops_schedules (
		id              TEXT PRIMARY KEY,
		runbook_id      TEXT NOT NULL,
		name            TEXT NOT NULL DEFAULT '',
		schedule_type   TEXT NOT NULL,
		cron_expr       TEXT NOT NULL DEFAULT '',
		timezone        TEXT NOT NULL DEFAULT 'UTC',
		run_at          TEXT NOT NULL DEFAULT '',
		enabled         INTEGER NOT NULL DEFAULT 1,
		last_run_at     TEXT NOT NULL DEFAULT '',
		last_run_status TEXT NOT NULL DEFAULT '',
		next_run_at     TEXT NOT NULL DEFAULT '',
		created_at      TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_ops_schedules_next_run
		ON ops_schedules (enabled, next_run_at ASC);
	CREATE INDEX IF NOT EXISTS idx_ops_schedules_runbook
		ON ops_schedules (runbook_id)`

	_, err := s.db.Exec(schema)
	return err
}

// ListOpsSchedules returns all schedules ordered by name.
func (s *Store) ListOpsSchedules(ctx context.Context) ([]OpsSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, runbook_id, name, schedule_type, cron_expr, timezone,
		        run_at, enabled, last_run_at, last_run_status, next_run_at,
		        created_at, updated_at
		 FROM ops_schedules ORDER BY name ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanOpsSchedules(rows)
}

// ListDueSchedules returns enabled schedules whose next_run_at <= now.
func (s *Store) ListDueSchedules(ctx context.Context, now time.Time) ([]OpsSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, runbook_id, name, schedule_type, cron_expr, timezone,
		        run_at, enabled, last_run_at, last_run_status, next_run_at,
		        created_at, updated_at
		 FROM ops_schedules
		 WHERE enabled = 1 AND next_run_at != '' AND next_run_at <= ?
		 ORDER BY next_run_at ASC`,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanOpsSchedules(rows)
}

// ListSchedulesByRunbook returns schedules for a specific runbook.
func (s *Store) ListSchedulesByRunbook(ctx context.Context, runbookID string) ([]OpsSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, runbook_id, name, schedule_type, cron_expr, timezone,
		        run_at, enabled, last_run_at, last_run_status, next_run_at,
		        created_at, updated_at
		 FROM ops_schedules WHERE runbook_id = ?
		 ORDER BY created_at ASC`, runbookID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanOpsSchedules(rows)
}

// InsertOpsSchedule creates a new schedule.
func (s *Store) InsertOpsSchedule(ctx context.Context, w OpsScheduleWrite) (OpsSchedule, error) {
	id := w.ID
	if id == "" {
		id = randomScheduleID()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ops_schedules
		 (id, runbook_id, name, schedule_type, cron_expr, timezone, run_at, enabled, next_run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, w.RunbookID, w.Name, w.ScheduleType, w.CronExpr, w.Timezone,
		w.RunAt, boolToInt(w.Enabled), w.NextRunAt)
	if err != nil {
		return OpsSchedule{}, err
	}
	return s.getOpsScheduleByID(ctx, id)
}

// UpdateOpsSchedule updates an existing schedule.
func (s *Store) UpdateOpsSchedule(ctx context.Context, w OpsScheduleWrite) (OpsSchedule, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE ops_schedules SET
		 name = ?, schedule_type = ?, cron_expr = ?, timezone = ?,
		 run_at = ?, enabled = ?, next_run_at = ?,
		 updated_at = datetime('now')
		 WHERE id = ?`,
		w.Name, w.ScheduleType, w.CronExpr, w.Timezone,
		w.RunAt, boolToInt(w.Enabled), w.NextRunAt, w.ID)
	if err != nil {
		return OpsSchedule{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return OpsSchedule{}, sql.ErrNoRows
	}
	return s.getOpsScheduleByID(ctx, w.ID)
}

// DeleteOpsSchedule removes a schedule by ID.
func (s *Store) DeleteOpsSchedule(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM ops_schedules WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateScheduleAfterRun updates the schedule's last run info and next run time.
func (s *Store) UpdateScheduleAfterRun(ctx context.Context, id, lastRunAt, lastRunStatus, nextRunAt string, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ops_schedules SET
		 last_run_at = ?, last_run_status = ?, next_run_at = ?,
		 enabled = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		lastRunAt, lastRunStatus, nextRunAt, boolToInt(enabled), id)
	return err
}

// DeleteSchedulesByRunbook removes all schedules for a runbook.
func (s *Store) DeleteSchedulesByRunbook(ctx context.Context, runbookID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM ops_schedules WHERE runbook_id = ?", runbookID)
	return err
}

func (s *Store) getOpsScheduleByID(ctx context.Context, id string) (OpsSchedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, runbook_id, name, schedule_type, cron_expr, timezone,
		        run_at, enabled, last_run_at, last_run_status, next_run_at,
		        created_at, updated_at
		 FROM ops_schedules WHERE id = ?`, id)
	return scanOpsSchedule(row)
}

func scanOpsSchedules(rows *sql.Rows) ([]OpsSchedule, error) {
	var out []OpsSchedule
	for rows.Next() {
		var sched OpsSchedule
		var enabled int
		if err := rows.Scan(
			&sched.ID, &sched.RunbookID, &sched.Name,
			&sched.ScheduleType, &sched.CronExpr, &sched.Timezone,
			&sched.RunAt, &enabled, &sched.LastRunAt, &sched.LastRunStatus,
			&sched.NextRunAt, &sched.CreatedAt, &sched.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sched.Enabled = enabled != 0
		out = append(out, sched)
	}
	return out, rows.Err()
}

type opsScheduleRowScanner interface {
	Scan(dest ...any) error
}

func scanOpsSchedule(row opsScheduleRowScanner) (OpsSchedule, error) {
	var sched OpsSchedule
	var enabled int
	if err := row.Scan(
		&sched.ID, &sched.RunbookID, &sched.Name,
		&sched.ScheduleType, &sched.CronExpr, &sched.Timezone,
		&sched.RunAt, &enabled, &sched.LastRunAt, &sched.LastRunStatus,
		&sched.NextRunAt, &sched.CreatedAt, &sched.UpdatedAt,
	); err != nil {
		return OpsSchedule{}, err
	}
	sched.Enabled = enabled != 0
	return sched, nil
}

func randomScheduleID() string {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("sched-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
