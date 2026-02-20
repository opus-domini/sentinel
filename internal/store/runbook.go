package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	opsRunbookStatusQueued    = "queued"
	opsRunbookStatusRunning   = "running"
	opsRunbookStatusSucceeded = "succeeded"
	opsRunbookStatusFailed    = "failed"

	opsRunbookOrphanError = "interrupted by server restart"
)

type OpsRunbookStep struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Command     string `json:"command,omitempty"`
	Check       string `json:"check,omitempty"`
	Description string `json:"description,omitempty"`
}

type OpsRunbook struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Enabled     bool             `json:"enabled"`
	Steps       []OpsRunbookStep `json:"steps"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
}

type OpsRunbookStepResult struct {
	StepIndex  int    `json:"stepIndex"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	DurationMs int64  `json:"durationMs"`
}

type OpsRunbookRun struct {
	ID             string                 `json:"id"`
	RunbookID      string                 `json:"runbookId"`
	RunbookName    string                 `json:"runbookName"`
	Status         string                 `json:"status"`
	TotalSteps     int                    `json:"totalSteps"`
	CompletedSteps int                    `json:"completedSteps"`
	CurrentStep    string                 `json:"currentStep"`
	Error          string                 `json:"error"`
	StepResults    []OpsRunbookStepResult `json:"stepResults"`
	CreatedAt      string                 `json:"createdAt"`
	StartedAt      string                 `json:"startedAt,omitempty"`
	FinishedAt     string                 `json:"finishedAt,omitempty"`
}

type OpsRunbookWrite struct {
	ID          string
	Name        string
	Description string
	Steps       []OpsRunbookStep
	Enabled     bool
}

type OpsRunbookRunUpdate struct {
	RunID          string
	Status         string
	CompletedSteps int
	CurrentStep    string
	Error          string
	StepResults    string
	StartedAt      string
	FinishedAt     string
}

func (s *Store) initRunbookSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ops_runbooks (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			steps_json  TEXT NOT NULL DEFAULT '[]',
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS ops_runbook_runs (
			id              TEXT PRIMARY KEY,
			runbook_id      TEXT NOT NULL,
			runbook_name    TEXT NOT NULL,
			status          TEXT NOT NULL,
			total_steps     INTEGER NOT NULL DEFAULT 0,
			completed_steps INTEGER NOT NULL DEFAULT 0,
			current_step    TEXT NOT NULL DEFAULT '',
			error           TEXT NOT NULL DEFAULT '',
			step_results    TEXT NOT NULL DEFAULT '[]',
			created_at      TEXT NOT NULL,
			started_at      TEXT NOT NULL DEFAULT '',
			finished_at     TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_created
			ON ops_runbook_runs (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_status
			ON ops_runbook_runs (status, created_at DESC, id DESC)`,
		`INSERT OR IGNORE INTO ops_runbooks(
			id, name, description, steps_json, enabled, created_at, updated_at
		) VALUES (
			'ops.service.recover',
			'Service Recovery',
			'Validate and recover the Sentinel service runtime.',
			'[{"type":"command","title":"Inspect service status","command":"sentinel service status"},{"type":"command","title":"Restart service","command":"sentinel service install --start=true"},{"type":"check","title":"Confirm healthy status","check":"service should be active"}]',
			1,
			datetime('now'),
			datetime('now')
		)`,
		`INSERT OR IGNORE INTO ops_runbooks(
			id, name, description, steps_json, enabled, created_at, updated_at
		) VALUES (
			'ops.autoupdate.verify',
			'Autoupdate Verification',
			'Check updater configuration and latest release state.',
			'[{"type":"command","title":"Check updater timer","command":"sentinel service autoupdate status"},{"type":"command","title":"Check release status","command":"sentinel update check"},{"type":"manual","title":"Review output","description":"Review versions and update policy before apply."}]',
			1,
			datetime('now'),
			datetime('now')
		)`,
		`INSERT OR IGNORE INTO ops_runbooks(
			id, name, description, steps_json, enabled, created_at, updated_at
		) VALUES (
			'ops.update.apply',
			'Apply Update',
			'Check for updates, download and install the latest version, and restart the service.',
			'[{"type":"command","title":"Check for updates","command":"sentinel update check"},{"type":"command","title":"Apply update and restart","command":"sentinel update apply --restart"}]',
			1,
			datetime('now'),
			datetime('now')
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListOpsRunbooks(ctx context.Context) ([]OpsRunbook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, name, description, steps_json, enabled, created_at, updated_at
	FROM ops_runbooks
	ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runbooks := make([]OpsRunbook, 0, 8)
	for rows.Next() {
		var (
			item      OpsRunbook
			stepsJSON string
			enabled   int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&stepsJSON,
			&enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(stepsJSON), &item.Steps); err != nil {
			item.Steps = []OpsRunbookStep{}
		}
		runbooks = append(runbooks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runbooks, nil
}

func (s *Store) StartOpsRunbook(ctx context.Context, runbookID string, at time.Time) (OpsRunbookRun, error) {
	runbookID = strings.TrimSpace(runbookID)
	if runbookID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}

	runbook, err := s.getOpsRunbookByID(ctx, runbookID)
	if err != nil {
		return OpsRunbookRun{}, err
	}

	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := uuid.NewString()
	totalSteps := len(runbook.Steps)
	currentStep := ""
	if totalSteps > 0 {
		currentStep = runbook.Steps[0].Title
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, '', '[]', ?, '', '')`,
		runID,
		runbook.ID,
		runbook.Name,
		opsRunbookStatusQueued,
		totalSteps,
		0,
		currentStep,
		now.Format(time.RFC3339),
	); err != nil {
		return OpsRunbookRun{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE ops_runbook_runs
		SET status = ?, started_at = ?, current_step = ?
		WHERE id = ?`,
		opsRunbookStatusRunning,
		now.Format(time.RFC3339),
		currentStep,
		runID,
	); err != nil {
		return OpsRunbookRun{}, err
	}

	finalStep := "completed"
	if totalSteps > 0 {
		finalStep = runbook.Steps[totalSteps-1].Title
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ops_runbook_runs
		SET status = ?, completed_steps = ?, current_step = ?, finished_at = ?
		WHERE id = ?`,
		opsRunbookStatusSucceeded,
		totalSteps,
		finalStep,
		now.Format(time.RFC3339),
		runID,
	); err != nil {
		return OpsRunbookRun{}, err
	}

	if err := tx.Commit(); err != nil {
		return OpsRunbookRun{}, err
	}

	return s.GetOpsRunbookRun(ctx, runID)
}

func (s *Store) ListOpsRunbookRuns(ctx context.Context, limit int) ([]OpsRunbookRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, created_at, started_at, finished_at
	FROM ops_runbook_runs
	ORDER BY created_at DESC, id DESC
	LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]OpsRunbookRun, 0, limit)
	for rows.Next() {
		item, err := scanOpsRunbookRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetOpsRunbookRun(ctx context.Context, runID string) (OpsRunbookRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, created_at, started_at, finished_at
	FROM ops_runbook_runs
	WHERE id = ?
	LIMIT 1`, runID)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	item, err := scanOpsRunbookRun(rows)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	if err := rows.Err(); err != nil {
		return OpsRunbookRun{}, err
	}
	return item, nil
}

// GetOpsRunbook returns a single runbook by ID using a direct DB lookup.
func (s *Store) GetOpsRunbook(ctx context.Context, id string) (OpsRunbook, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return OpsRunbook{}, sql.ErrNoRows
	}
	return s.getOpsRunbookByID(ctx, id)
}

func (s *Store) getOpsRunbookByID(ctx context.Context, runbookID string) (OpsRunbook, error) {
	var (
		out      OpsRunbook
		stepsRaw string
		enabled  int
	)
	err := s.db.QueryRowContext(ctx, `SELECT
		id, name, description, steps_json, enabled, created_at, updated_at
	FROM ops_runbooks
	WHERE id = ?`, runbookID).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&stepsRaw,
		&enabled,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return OpsRunbook{}, err
	}
	out.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(stepsRaw), &out.Steps); err != nil {
		out.Steps = []OpsRunbookStep{}
	}
	return out, nil
}

type opsRunbookRunScanner interface {
	Scan(dest ...any) error
}

func scanOpsRunbookRun(scanner opsRunbookRunScanner) (OpsRunbookRun, error) {
	var (
		out            OpsRunbookRun
		stepResultsRaw string
	)
	if err := scanner.Scan(
		&out.ID,
		&out.RunbookID,
		&out.RunbookName,
		&out.Status,
		&out.TotalSteps,
		&out.CompletedSteps,
		&out.CurrentStep,
		&out.Error,
		&stepResultsRaw,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	); err != nil {
		return OpsRunbookRun{}, err
	}
	if err := json.Unmarshal([]byte(stepResultsRaw), &out.StepResults); err != nil || out.StepResults == nil {
		out.StepResults = []OpsRunbookStepResult{}
	}
	return out, nil
}

func (s *Store) InsertOpsRunbook(ctx context.Context, w OpsRunbookWrite) (OpsRunbook, error) {
	id := strings.TrimSpace(w.ID)
	if id == "" {
		id = uuid.NewString()
	}
	name := strings.TrimSpace(w.Name)
	if name == "" {
		return OpsRunbook{}, fmt.Errorf("runbook name is required")
	}
	steps := w.Steps
	if steps == nil {
		steps = []OpsRunbookStep{}
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return OpsRunbook{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if w.Enabled {
		enabled = 1
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_runbooks (
		id, name, description, steps_json, enabled, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, strings.TrimSpace(w.Description), string(stepsJSON), enabled, now, now,
	); err != nil {
		return OpsRunbook{}, err
	}
	return s.getOpsRunbookByID(ctx, id)
}

func (s *Store) UpdateOpsRunbook(ctx context.Context, w OpsRunbookWrite) (OpsRunbook, error) {
	id := strings.TrimSpace(w.ID)
	if id == "" {
		return OpsRunbook{}, sql.ErrNoRows
	}
	name := strings.TrimSpace(w.Name)
	if name == "" {
		return OpsRunbook{}, fmt.Errorf("runbook name is required")
	}
	steps := w.Steps
	if steps == nil {
		steps = []OpsRunbookStep{}
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return OpsRunbook{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if w.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE ops_runbooks SET
		name = ?, description = ?, steps_json = ?, enabled = ?, updated_at = ?
	WHERE id = ?`,
		name, strings.TrimSpace(w.Description), string(stepsJSON), enabled, now, id,
	)
	if err != nil {
		return OpsRunbook{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return OpsRunbook{}, err
	}
	if affected == 0 {
		return OpsRunbook{}, sql.ErrNoRows
	}
	return s.getOpsRunbookByID(ctx, id)
}

func (s *Store) DeleteOpsRunbook(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM ops_runbooks WHERE id = ?", id)
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

func (s *Store) UpdateOpsRunbookRun(ctx context.Context, u OpsRunbookRunUpdate) (OpsRunbookRun, error) {
	runID := strings.TrimSpace(u.RunID)
	if runID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	stepResults := strings.TrimSpace(u.StepResults)
	if stepResults == "" {
		stepResults = "[]"
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE ops_runbook_runs SET
		status = ?, completed_steps = ?, current_step = ?, error = ?, step_results = ?, started_at = ?, finished_at = ?
	WHERE id = ?`,
		strings.TrimSpace(u.Status),
		u.CompletedSteps,
		strings.TrimSpace(u.CurrentStep),
		strings.TrimSpace(u.Error),
		stepResults,
		strings.TrimSpace(u.StartedAt),
		strings.TrimSpace(u.FinishedAt),
		runID,
	); err != nil {
		return OpsRunbookRun{}, err
	}
	return s.GetOpsRunbookRun(ctx, runID)
}

func (s *Store) CreateOpsRunbookRun(ctx context.Context, runbookID string, at time.Time) (OpsRunbookRun, error) {
	runbookID = strings.TrimSpace(runbookID)
	if runbookID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	runbook, err := s.getOpsRunbookByID(ctx, runbookID)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := uuid.NewString()
	totalSteps := len(runbook.Steps)
	currentStep := ""
	if totalSteps > 0 {
		currentStep = runbook.Steps[0].Title
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, 0, ?, '', '[]', ?, '', '')`,
		runID, runbook.ID, runbook.Name, opsRunbookStatusQueued, totalSteps, currentStep, now.Format(time.RFC3339),
	); err != nil {
		return OpsRunbookRun{}, err
	}
	return s.GetOpsRunbookRun(ctx, runID)
}

func (s *Store) FailOrphanedRuns(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE ops_runbook_runs
			SET status = ?, error = ?, finished_at = ?
		  WHERE status IN (?, ?)`,
		opsRunbookStatusFailed, opsRunbookOrphanError, now,
		opsRunbookStatusRunning, opsRunbookStatusQueued,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) DeleteOpsRunbookRun(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM ops_runbook_runs WHERE id = ?", runID)
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
