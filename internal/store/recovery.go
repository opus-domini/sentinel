package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type RecoverySessionState string

const (
	RecoveryStateRunning   RecoverySessionState = "running"
	RecoveryStateKilled    RecoverySessionState = "killed"
	RecoveryStateRestoring RecoverySessionState = "restoring"
	RecoveryStateRestored  RecoverySessionState = "restored"
	RecoveryStateArchived  RecoverySessionState = "archived"
)

type RecoverySession struct {
	Name             string               `json:"name"`
	State            RecoverySessionState `json:"state"`
	LatestSnapshotID int64                `json:"latestSnapshotId"`
	SnapshotHash     string               `json:"snapshotHash"`
	SnapshotAt       time.Time            `json:"snapshotAt"`
	LastBootID       string               `json:"lastBootId"`
	LastSeenAt       time.Time            `json:"lastSeenAt"`
	KilledAt         *time.Time           `json:"killedAt,omitempty"`
	RestoredAt       *time.Time           `json:"restoredAt,omitempty"`
	ArchivedAt       *time.Time           `json:"archivedAt,omitempty"`
	RestoreError     string               `json:"restoreError"`
	Windows          int                  `json:"windows"`
	Panes            int                  `json:"panes"`
}

type RecoverySnapshotWrite struct {
	SessionName  string
	BootID       string
	StateHash    string
	CapturedAt   time.Time
	ActiveWindow int
	ActivePaneID string
	Windows      int
	Panes        int
	PayloadJSON  string
}

type RecoverySnapshot struct {
	ID           int64     `json:"id"`
	SessionName  string    `json:"sessionName"`
	BootID       string    `json:"bootId"`
	StateHash    string    `json:"stateHash"`
	CapturedAt   time.Time `json:"capturedAt"`
	ActiveWindow int       `json:"activeWindow"`
	ActivePaneID string    `json:"activePaneId"`
	Windows      int       `json:"windows"`
	Panes        int       `json:"panes"`
	PayloadJSON  string    `json:"payloadJson"`
}

type RecoveryJobStatus string

const (
	RecoveryJobQueued    RecoveryJobStatus = "queued"
	RecoveryJobRunning   RecoveryJobStatus = "running"
	RecoveryJobSucceeded RecoveryJobStatus = "succeeded"
	RecoveryJobFailed    RecoveryJobStatus = "failed"
	RecoveryJobPartial   RecoveryJobStatus = "partial"
)

type RecoveryJob struct {
	ID             string            `json:"id"`
	SessionName    string            `json:"sessionName"`
	TargetSession  string            `json:"targetSession"`
	SnapshotID     int64             `json:"snapshotId"`
	Mode           string            `json:"mode"`
	ConflictPolicy string            `json:"conflictPolicy"`
	Status         RecoveryJobStatus `json:"status"`
	TotalSteps     int               `json:"totalSteps"`
	CompletedSteps int               `json:"completedSteps"`
	CurrentStep    string            `json:"currentStep"`
	Error          string            `json:"error"`
	CreatedAt      time.Time         `json:"createdAt"`
	StartedAt      *time.Time        `json:"startedAt,omitempty"`
	FinishedAt     *time.Time        `json:"finishedAt,omitempty"`
}

func (s *Store) initRecoverySchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS recovery_sessions (
			name                TEXT PRIMARY KEY,
			state               TEXT NOT NULL DEFAULT 'running',
			latest_snapshot_id  INTEGER NOT NULL DEFAULT 0,
			snapshot_hash       TEXT NOT NULL DEFAULT '',
			snapshot_at         TEXT NOT NULL DEFAULT '',
			last_boot_id        TEXT NOT NULL DEFAULT '',
			last_seen_at        TEXT NOT NULL DEFAULT '',
			killed_at           TEXT NOT NULL DEFAULT '',
			restored_at         TEXT NOT NULL DEFAULT '',
			archived_at         TEXT NOT NULL DEFAULT '',
			restore_error       TEXT NOT NULL DEFAULT '',
			windows             INTEGER NOT NULL DEFAULT 0,
			panes               INTEGER NOT NULL DEFAULT 0,
			updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS recovery_snapshots (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			session_name  TEXT NOT NULL,
			boot_id       TEXT NOT NULL,
			state_hash    TEXT NOT NULL,
			captured_at   TEXT NOT NULL,
			active_window INTEGER NOT NULL DEFAULT 0,
			active_pane_id TEXT NOT NULL DEFAULT '',
			windows       INTEGER NOT NULL DEFAULT 0,
			panes         INTEGER NOT NULL DEFAULT 0,
			payload_json  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recovery_snapshots_session_captured_at
			ON recovery_snapshots (session_name, captured_at DESC)`,
		`CREATE TABLE IF NOT EXISTS recovery_jobs (
			id               TEXT PRIMARY KEY,
			session_name     TEXT NOT NULL,
			target_session   TEXT NOT NULL DEFAULT '',
			snapshot_id      INTEGER NOT NULL,
			mode             TEXT NOT NULL,
			conflict_policy  TEXT NOT NULL,
			status           TEXT NOT NULL,
			total_steps      INTEGER NOT NULL DEFAULT 0,
			completed_steps  INTEGER NOT NULL DEFAULT 0,
			current_step     TEXT NOT NULL DEFAULT '',
			error            TEXT NOT NULL DEFAULT '',
			created_at       TEXT NOT NULL,
			started_at       TEXT NOT NULL DEFAULT '',
			finished_at      TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recovery_jobs_created_at
			ON recovery_jobs (created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS runtime_kv (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertRecoverySnapshot(ctx context.Context, snap RecoverySnapshotWrite) (RecoverySnapshot, bool, error) {
	if strings.TrimSpace(snap.SessionName) == "" {
		return RecoverySnapshot{}, false, errors.New("session name is required")
	}
	if strings.TrimSpace(snap.PayloadJSON) == "" {
		return RecoverySnapshot{}, false, errors.New("payload json is required")
	}
	if !json.Valid([]byte(snap.PayloadJSON)) {
		return RecoverySnapshot{}, false, errors.New("payload json must be valid JSON")
	}

	capturedAt := snap.CapturedAt.UTC()
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	stateHash := strings.TrimSpace(snap.StateHash)
	bootID := strings.TrimSpace(snap.BootID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecoverySnapshot{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		prevSnapshotID int64
		prevHash       string
	)
	switch err := tx.QueryRowContext(ctx,
		`SELECT latest_snapshot_id, snapshot_hash
		   FROM recovery_sessions
		  WHERE name = ?`,
		snap.SessionName,
	).Scan(&prevSnapshotID, &prevHash); {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		prevSnapshotID = 0
		prevHash = ""
	default:
		return RecoverySnapshot{}, false, err
	}

	// If nothing changed, keep the same snapshot row and only refresh liveness.
	if prevSnapshotID > 0 && prevHash == stateHash && stateHash != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE recovery_sessions
			    SET state = ?,
			        snapshot_at = ?,
			        last_boot_id = ?,
			        last_seen_at = ?,
			        restore_error = '',
			        windows = ?,
			        panes = ?,
			        updated_at = datetime('now')
			  WHERE name = ?`,
			RecoveryStateRunning,
			capturedAt.Format(time.RFC3339),
			bootID,
			capturedAt.Format(time.RFC3339),
			snap.Windows,
			snap.Panes,
			snap.SessionName,
		); err != nil {
			return RecoverySnapshot{}, false, err
		}
		row, err := queryRecoverySnapshotTx(ctx, tx, prevSnapshotID)
		if err != nil {
			return RecoverySnapshot{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return RecoverySnapshot{}, false, err
		}
		return row, false, nil
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO recovery_snapshots (
			session_name, boot_id, state_hash, captured_at,
			active_window, active_pane_id, windows, panes, payload_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.SessionName,
		bootID,
		stateHash,
		capturedAt.Format(time.RFC3339),
		snap.ActiveWindow,
		snap.ActivePaneID,
		snap.Windows,
		snap.Panes,
		snap.PayloadJSON,
	)
	if err != nil {
		return RecoverySnapshot{}, false, err
	}
	newID, err := result.LastInsertId()
	if err != nil {
		return RecoverySnapshot{}, false, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO recovery_sessions (
			name, state, latest_snapshot_id, snapshot_hash, snapshot_at,
			last_boot_id, last_seen_at, killed_at, restored_at, archived_at,
			restore_error, windows, panes, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', '', ?, ?, datetime('now'))
		ON CONFLICT(name) DO UPDATE SET
			state = excluded.state,
			latest_snapshot_id = excluded.latest_snapshot_id,
			snapshot_hash = excluded.snapshot_hash,
			snapshot_at = excluded.snapshot_at,
			last_boot_id = excluded.last_boot_id,
			last_seen_at = excluded.last_seen_at,
			killed_at = '',
			restored_at = '',
			archived_at = '',
			restore_error = '',
			windows = excluded.windows,
			panes = excluded.panes,
			updated_at = excluded.updated_at`,
		snap.SessionName,
		RecoveryStateRunning,
		newID,
		stateHash,
		capturedAt.Format(time.RFC3339),
		bootID,
		capturedAt.Format(time.RFC3339),
		snap.Windows,
		snap.Panes,
	); err != nil {
		return RecoverySnapshot{}, false, err
	}

	row, err := queryRecoverySnapshotTx(ctx, tx, newID)
	if err != nil {
		return RecoverySnapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return RecoverySnapshot{}, false, err
	}
	return row, true, nil
}

func (s *Store) GetRecoverySnapshot(ctx context.Context, id int64) (RecoverySnapshot, error) {
	return queryRecoverySnapshotDB(ctx, s.db, id)
}

func (s *Store) ListRecoverySnapshots(ctx context.Context, sessionName string, limit int) ([]RecoverySnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_name, boot_id, state_hash, captured_at, active_window, active_pane_id, windows, panes, payload_json
		   FROM recovery_snapshots
		  WHERE session_name = ?
		  ORDER BY captured_at DESC
		  LIMIT ?`,
		sessionName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []RecoverySnapshot
	for rows.Next() {
		row, err := scanRecoverySnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListRecoverySessions(ctx context.Context, states []RecoverySessionState) ([]RecoverySession, error) {
	query := `SELECT name, state, latest_snapshot_id, snapshot_hash, snapshot_at, last_boot_id, last_seen_at,
	                 killed_at, restored_at, archived_at, restore_error, windows, panes
	            FROM recovery_sessions`
	args := make([]any, 0, len(states))
	if len(states) > 0 {
		placeholders := make([]string, len(states))
		for i, state := range states {
			placeholders[i] = "?"
			args = append(args, string(state))
		}
		query += " WHERE state IN (" + strings.Join(placeholders, ", ") + ")" //nolint:gosec // placeholders are static
	}
	query += " ORDER BY CASE state WHEN 'killed' THEN 0 WHEN 'restoring' THEN 1 WHEN 'running' THEN 2 WHEN 'restored' THEN 3 ELSE 4 END, snapshot_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]RecoverySession, 0, 16)
	for rows.Next() {
		var (
			row                        RecoverySession
			stateRaw                   string
			snapshotAtRaw, lastSeenRaw string
			killedRaw, restoredRaw     string
			archivedRaw, restoreError  string
		)
		if err := rows.Scan(
			&row.Name,
			&stateRaw,
			&row.LatestSnapshotID,
			&row.SnapshotHash,
			&snapshotAtRaw,
			&row.LastBootID,
			&lastSeenRaw,
			&killedRaw,
			&restoredRaw,
			&archivedRaw,
			&restoreError,
			&row.Windows,
			&row.Panes,
		); err != nil {
			return nil, err
		}
		row.State = RecoverySessionState(stateRaw)
		row.SnapshotAt = parseStoreTime(snapshotAtRaw)
		row.LastSeenAt = parseStoreTime(lastSeenRaw)
		row.KilledAt = parseStoreTimePtr(killedRaw)
		row.RestoredAt = parseStoreTimePtr(restoredRaw)
		row.ArchivedAt = parseStoreTimePtr(archivedRaw)
		row.RestoreError = restoreError
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) GetRecoverySession(ctx context.Context, name string) (RecoverySession, error) {
	var (
		row                                         RecoverySession
		stateRaw, snapshotAtRaw, lastSeenRaw        string
		killedRaw, restoredRaw, archivedRaw, errRaw string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT name, state, latest_snapshot_id, snapshot_hash, snapshot_at, last_boot_id, last_seen_at,
		        killed_at, restored_at, archived_at, restore_error, windows, panes
		   FROM recovery_sessions
		  WHERE name = ?`,
		name,
	).Scan(
		&row.Name,
		&stateRaw,
		&row.LatestSnapshotID,
		&row.SnapshotHash,
		&snapshotAtRaw,
		&row.LastBootID,
		&lastSeenRaw,
		&killedRaw,
		&restoredRaw,
		&archivedRaw,
		&errRaw,
		&row.Windows,
		&row.Panes,
	)
	if err != nil {
		return RecoverySession{}, err
	}
	row.State = RecoverySessionState(stateRaw)
	row.SnapshotAt = parseStoreTime(snapshotAtRaw)
	row.LastSeenAt = parseStoreTime(lastSeenRaw)
	row.KilledAt = parseStoreTimePtr(killedRaw)
	row.RestoredAt = parseStoreTimePtr(restoredRaw)
	row.ArchivedAt = parseStoreTimePtr(archivedRaw)
	row.RestoreError = errRaw
	return row, nil
}

func (s *Store) MarkRecoverySessionsKilled(ctx context.Context, names []string, bootID string, killedAt time.Time) error {
	if len(names) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	when := killedAt.UTC().Format(time.RFC3339)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE recovery_sessions
			    SET state = ?,
			        killed_at = ?,
			        last_seen_at = ?,
			        last_boot_id = ?,
			        restore_error = '',
			        updated_at = datetime('now')
			  WHERE name = ?
			    AND state IN (?, ?, ?)`,
			RecoveryStateKilled,
			when,
			when,
			bootID,
			name,
			RecoveryStateRunning,
			RecoveryStateRestored,
			RecoveryStateRestoring,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) RenameRecoverySession(ctx context.Context, oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE recovery_sessions
		    SET name = ?,
		        updated_at = datetime('now')
		  WHERE name = ?`,
		newName,
		oldName,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE recovery_snapshots
		    SET session_name = ?
		  WHERE session_name = ?`,
		newName,
		oldName,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE recovery_jobs
		    SET session_name = ?
		  WHERE session_name = ?`,
		newName,
		oldName,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkRecoverySessionArchived(ctx context.Context, name string, archivedAt time.Time) error {
	when := archivedAt.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_sessions
		    SET state = ?,
		        archived_at = ?,
		        updated_at = datetime('now')
		  WHERE name = ?`,
		RecoveryStateArchived,
		when,
		name,
	)
	return err
}

func (s *Store) MarkRecoverySessionRestoring(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_sessions
		    SET state = ?,
		        restore_error = '',
		        updated_at = datetime('now')
		  WHERE name = ?`,
		RecoveryStateRestoring,
		name,
	)
	return err
}

func (s *Store) MarkRecoverySessionRestored(ctx context.Context, name string, restoredAt time.Time) error {
	when := restoredAt.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_sessions
		    SET state = ?,
		        restored_at = ?,
		        killed_at = '',
		        restore_error = '',
		        updated_at = datetime('now')
		  WHERE name = ?`,
		RecoveryStateRestored,
		when,
		name,
	)
	return err
}

func (s *Store) MarkRecoverySessionRestoreFailed(ctx context.Context, name, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_sessions
		    SET state = ?,
		        restore_error = ?,
		        updated_at = datetime('now')
		  WHERE name = ?`,
		RecoveryStateKilled,
		strings.TrimSpace(errMsg),
		name,
	)
	return err
}

func (s *Store) TrimRecoverySnapshots(ctx context.Context, maxPerSession int) error {
	if maxPerSession <= 0 {
		return nil
	}
	sessions, err := s.ListRecoverySessions(ctx, nil)
	if err != nil {
		return err
	}
	for _, item := range sessions {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id
			   FROM recovery_snapshots
			  WHERE session_name = ?
			  ORDER BY captured_at DESC
			  LIMIT -1 OFFSET ?`,
			item.Name,
			maxPerSession,
		)
		if err != nil {
			return err
		}
		var staleIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			staleIDs = append(staleIDs, id)
		}
		_ = rows.Close()
		if len(staleIDs) == 0 {
			continue
		}
		placeholders := make([]string, len(staleIDs))
		args := make([]any, len(staleIDs))
		for i, id := range staleIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		query := "DELETE FROM recovery_snapshots WHERE id IN (" + strings.Join(placeholders, ", ") + ")" //nolint:gosec // placeholders are literals
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetRuntimeValue(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runtime_kv (key, value, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET
		   value = excluded.value,
		   updated_at = excluded.updated_at`,
		key, value,
	)
	return err
}

func (s *Store) GetRuntimeValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM runtime_kv WHERE key = ?`,
		key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) CreateRecoveryJob(ctx context.Context, job RecoveryJob) error {
	createdAt := job.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO recovery_jobs (
			id, session_name, target_session, snapshot_id, mode, conflict_policy, status,
			total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID,
		job.SessionName,
		job.TargetSession,
		job.SnapshotID,
		job.Mode,
		job.ConflictPolicy,
		job.Status,
		job.TotalSteps,
		job.CompletedSteps,
		job.CurrentStep,
		strings.TrimSpace(job.Error),
		createdAt.Format(time.RFC3339),
		formatTimePtr(job.StartedAt),
		formatTimePtr(job.FinishedAt),
	)
	return err
}

func (s *Store) SetRecoveryJobRunning(ctx context.Context, id string, startedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_jobs
		    SET status = ?,
		        started_at = ?,
		        current_step = '',
		        error = ''
		  WHERE id = ?`,
		RecoveryJobRunning,
		startedAt.UTC().Format(time.RFC3339),
		id,
	)
	return err
}

func (s *Store) UpdateRecoveryJobTarget(ctx context.Context, id, target string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_jobs
		    SET target_session = ?
		  WHERE id = ?`,
		strings.TrimSpace(target),
		id,
	)
	return err
}

func (s *Store) UpdateRecoveryJobProgress(ctx context.Context, id string, completedSteps, totalSteps int, currentStep string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_jobs
		    SET completed_steps = ?,
		        total_steps = ?,
		        current_step = ?
		  WHERE id = ?`,
		completedSteps,
		totalSteps,
		strings.TrimSpace(currentStep),
		id,
	)
	return err
}

func (s *Store) FinishRecoveryJob(ctx context.Context, id string, status RecoveryJobStatus, errMsg string, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recovery_jobs
		    SET status = ?,
		        error = ?,
		        current_step = '',
		        finished_at = ?
		  WHERE id = ?`,
		status,
		strings.TrimSpace(errMsg),
		finishedAt.UTC().Format(time.RFC3339),
		id,
	)
	return err
}

func (s *Store) GetRecoveryJob(ctx context.Context, id string) (RecoveryJob, error) {
	var (
		row                         RecoveryJob
		statusRaw, createdAtRaw     string
		startedAtRaw, finishedAtRaw string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, session_name, target_session, snapshot_id, mode, conflict_policy, status,
		        total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
		   FROM recovery_jobs
		  WHERE id = ?`,
		id,
	).Scan(
		&row.ID,
		&row.SessionName,
		&row.TargetSession,
		&row.SnapshotID,
		&row.Mode,
		&row.ConflictPolicy,
		&statusRaw,
		&row.TotalSteps,
		&row.CompletedSteps,
		&row.CurrentStep,
		&row.Error,
		&createdAtRaw,
		&startedAtRaw,
		&finishedAtRaw,
	)
	if err != nil {
		return RecoveryJob{}, err
	}
	row.Status = RecoveryJobStatus(statusRaw)
	row.CreatedAt = parseStoreTime(createdAtRaw)
	row.StartedAt = parseStoreTimePtr(startedAtRaw)
	row.FinishedAt = parseStoreTimePtr(finishedAtRaw)
	return row, nil
}

func (s *Store) ListRecoveryJobs(ctx context.Context, statuses []RecoveryJobStatus, limit int) ([]RecoveryJob, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, session_name, target_session, snapshot_id, mode, conflict_policy, status,
	                 total_steps, completed_steps, current_step, error, created_at, started_at, finished_at
	            FROM recovery_jobs`
	args := make([]any, 0, len(statuses)+1)
	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i, st := range statuses {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		query += " WHERE status IN (" + strings.Join(placeholders, ", ") + ")" //nolint:gosec // placeholders are static
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]RecoveryJob, 0, limit)
	for rows.Next() {
		var (
			row                        RecoveryJob
			statusRaw                  string
			createdAtRaw, startedAtRaw string
			finishedAtRaw              string
		)
		if err := rows.Scan(
			&row.ID,
			&row.SessionName,
			&row.TargetSession,
			&row.SnapshotID,
			&row.Mode,
			&row.ConflictPolicy,
			&statusRaw,
			&row.TotalSteps,
			&row.CompletedSteps,
			&row.CurrentStep,
			&row.Error,
			&createdAtRaw,
			&startedAtRaw,
			&finishedAtRaw,
		); err != nil {
			return nil, err
		}
		row.Status = RecoveryJobStatus(statusRaw)
		row.CreatedAt = parseStoreTime(createdAtRaw)
		row.StartedAt = parseStoreTimePtr(startedAtRaw)
		row.FinishedAt = parseStoreTimePtr(finishedAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryRecoverySnapshotDB(ctx context.Context, db *sql.DB, id int64) (RecoverySnapshot, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, session_name, boot_id, state_hash, captured_at, active_window, active_pane_id, windows, panes, payload_json
		   FROM recovery_snapshots
		  WHERE id = ?`,
		id,
	)
	return scanRecoverySnapshot(row)
}

func queryRecoverySnapshotTx(ctx context.Context, tx *sql.Tx, id int64) (RecoverySnapshot, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id, session_name, boot_id, state_hash, captured_at, active_window, active_pane_id, windows, panes, payload_json
		   FROM recovery_snapshots
		  WHERE id = ?`,
		id,
	)
	return scanRecoverySnapshot(row)
}

type scanRow interface {
	Scan(dest ...any) error
}

func scanRecoverySnapshot(src scanRow) (RecoverySnapshot, error) {
	var (
		row           RecoverySnapshot
		capturedAtRaw string
	)
	if err := src.Scan(
		&row.ID,
		&row.SessionName,
		&row.BootID,
		&row.StateHash,
		&capturedAtRaw,
		&row.ActiveWindow,
		&row.ActivePaneID,
		&row.Windows,
		&row.Panes,
		&row.PayloadJSON,
	); err != nil {
		return RecoverySnapshot{}, err
	}
	row.CapturedAt = parseStoreTime(capturedAtRaw)
	return row, nil
}

func parseStoreTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC()
	}
	if ts, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		return ts.UTC()
	}
	return time.Time{}
}

func parseStoreTimePtr(raw string) *time.Time {
	ts := parseStoreTime(raw)
	if ts.IsZero() {
		return nil
	}
	return &ts
}

func formatTimePtr(ts *time.Time) string {
	if ts == nil || ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
