package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

// RunbookParameter defines a single parameter that a runbook accepts.
// Parameters are substituted into step commands before execution.
type RunbookParameter struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // "string", "number", "boolean", "select"
	Default  string   `json:"default"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"` // for type "select"
}

type OpsRunbook struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Enabled     bool               `json:"enabled"`
	WebhookURL  string             `json:"webhookURL"`
	Steps       []OpsRunbookStep   `json:"steps"`
	Parameters  []RunbookParameter `json:"parameters"`
	CreatedAt   string             `json:"createdAt"`
	UpdatedAt   string             `json:"updatedAt"`
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
	ParametersUsed map[string]string      `json:"parametersUsed"`
	CreatedAt      string                 `json:"createdAt"`
	StartedAt      string                 `json:"startedAt,omitempty"`
	FinishedAt     string                 `json:"finishedAt,omitempty"`
}

type OpsRunbookWrite struct {
	ID          string
	Name        string
	Description string
	Steps       []OpsRunbookStep
	Parameters  []RunbookParameter
	Enabled     bool
	WebhookURL  string
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

func (s *Store) ListOpsRunbooks(ctx context.Context) ([]OpsRunbook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, name, description, steps_json, enabled, webhook_url, parameters, created_at, updated_at
	FROM ops_runbooks
	ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runbooks := make([]OpsRunbook, 0, 8)
	for rows.Next() {
		var (
			item       OpsRunbook
			stepsJSON  string
			paramsJSON string
			enabled    int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&stepsJSON,
			&enabled,
			&item.WebhookURL,
			&paramsJSON,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(stepsJSON), &item.Steps); err != nil {
			item.Steps = []OpsRunbookStep{}
		}
		if err := json.Unmarshal([]byte(paramsJSON), &item.Parameters); err != nil || item.Parameters == nil {
			item.Parameters = []RunbookParameter{}
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
	runID := randomID()
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
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, parameters_used, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, '', '[]', '{}', ?, '', '')`,
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
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, parameters_used, created_at, started_at, finished_at
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
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, parameters_used, created_at, started_at, finished_at
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
		out       OpsRunbook
		stepsRaw  string
		paramsRaw string
		enabled   int
	)
	err := s.db.QueryRowContext(ctx, `SELECT
		id, name, description, steps_json, enabled, webhook_url, parameters, created_at, updated_at
	FROM ops_runbooks
	WHERE id = ?`, runbookID).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&stepsRaw,
		&enabled,
		&out.WebhookURL,
		&paramsRaw,
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
	if err := json.Unmarshal([]byte(paramsRaw), &out.Parameters); err != nil || out.Parameters == nil {
		out.Parameters = []RunbookParameter{}
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
		paramsUsedRaw  string
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
		&paramsUsedRaw,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	); err != nil {
		return OpsRunbookRun{}, err
	}
	if err := json.Unmarshal([]byte(stepResultsRaw), &out.StepResults); err != nil || out.StepResults == nil {
		out.StepResults = []OpsRunbookStepResult{}
	}
	if err := json.Unmarshal([]byte(paramsUsedRaw), &out.ParametersUsed); err != nil || out.ParametersUsed == nil {
		out.ParametersUsed = map[string]string{}
	}
	return out, nil
}

func (s *Store) InsertOpsRunbook(ctx context.Context, w OpsRunbookWrite) (OpsRunbook, error) {
	id := strings.TrimSpace(w.ID)
	if id == "" {
		id = randomID()
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
	params := w.Parameters
	if params == nil {
		params = []RunbookParameter{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return OpsRunbook{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if w.Enabled {
		enabled = 1
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_runbooks (
		id, name, description, steps_json, enabled, webhook_url, parameters, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, strings.TrimSpace(w.Description), string(stepsJSON), enabled, strings.TrimSpace(w.WebhookURL), string(paramsJSON), now, now,
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
	params := w.Parameters
	if params == nil {
		params = []RunbookParameter{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return OpsRunbook{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if w.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE ops_runbooks SET
		name = ?, description = ?, steps_json = ?, enabled = ?, webhook_url = ?, parameters = ?, updated_at = ?
	WHERE id = ?`,
		name, strings.TrimSpace(w.Description), string(stepsJSON), enabled, strings.TrimSpace(w.WebhookURL), string(paramsJSON), now, id,
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
	startedAt := strings.TrimSpace(u.StartedAt)
	finishedAt := strings.TrimSpace(u.FinishedAt)
	if _, err := s.db.ExecContext(ctx, `UPDATE ops_runbook_runs SET
		status = ?,
		completed_steps = ?,
		current_step = ?,
		error = ?,
		step_results = ?,
		started_at = CASE WHEN ? != '' THEN ? ELSE started_at END,
		finished_at = CASE WHEN ? != '' THEN ? ELSE finished_at END
	WHERE id = ?`,
		strings.TrimSpace(u.Status),
		u.CompletedSteps,
		strings.TrimSpace(u.CurrentStep),
		strings.TrimSpace(u.Error),
		stepResults,
		startedAt, startedAt,
		finishedAt, finishedAt,
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
	runID := randomID()
	totalSteps := len(runbook.Steps)
	currentStep := ""
	if totalSteps > 0 {
		currentStep = runbook.Steps[0].Title
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, parameters_used, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, 0, ?, '', '[]', '{}', ?, '', '')`,
		runID, runbook.ID, runbook.Name, opsRunbookStatusQueued, totalSteps, currentStep, now.Format(time.RFC3339),
	); err != nil {
		return OpsRunbookRun{}, err
	}
	return s.GetOpsRunbookRun(ctx, runID)
}

// CreateOpsRunbookRunWithParams creates a new run record and stores the
// parameter values that were supplied by the caller.
func (s *Store) CreateOpsRunbookRunWithParams(ctx context.Context, runbookID string, at time.Time, params map[string]string) (OpsRunbookRun, error) {
	runbookID = strings.TrimSpace(runbookID)
	if runbookID == "" {
		return OpsRunbookRun{}, sql.ErrNoRows
	}
	rb, err := s.getOpsRunbookByID(ctx, runbookID)
	if err != nil {
		return OpsRunbookRun{}, err
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := randomID()
	totalSteps := len(rb.Steps)
	currentStep := ""
	if totalSteps > 0 {
		currentStep = rb.Steps[0].Title
	}
	if params == nil {
		params = map[string]string{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return OpsRunbookRun{}, fmt.Errorf("marshal parameters: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps, current_step, error, step_results, parameters_used, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, 0, ?, '', '[]', ?, ?, '', '')`,
		runID, rb.ID, rb.Name, opsRunbookStatusQueued, totalSteps, currentStep, string(paramsJSON), now.Format(time.RFC3339),
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

// SuggestRunbooksForMarker returns up to 5 enabled runbooks whose name or
// description contains the marker keyword or the session name. Results are
// ordered by relevance: name matches are ranked above description-only matches.
func (s *Store) SuggestRunbooksForMarker(ctx context.Context, marker, sessionName string) ([]OpsRunbook, error) {
	marker = strings.ToLower(strings.TrimSpace(marker))
	sessionName = strings.ToLower(strings.TrimSpace(sessionName))
	if marker == "" && sessionName == "" {
		return []OpsRunbook{}, nil
	}

	// Build a query that scores rows by match location (name > description).
	// We use CASE expressions to compute a relevance score and ORDER BY it.
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 8)

	if marker != "" {
		like := "%" + marker + "%"
		clauses = append(clauses, "(lower(name) LIKE ? OR lower(description) LIKE ?)")
		args = append(args, like, like)
	}
	if sessionName != "" {
		like := "%" + sessionName + "%"
		clauses = append(clauses, "(lower(name) LIKE ? OR lower(description) LIKE ?)")
		args = append(args, like, like)
	}

	where := "enabled = 1 AND (" + strings.Join(clauses, " OR ") + ")"

	// Relevance: name match on marker scores highest (1), name match on session (2),
	// description-only match (3).
	var scoreExpr string
	scoreArgs := make([]any, 0, 4)
	if marker != "" && sessionName != "" {
		markerLike := "%" + marker + "%"
		sessionLike := "%" + sessionName + "%"
		scoreExpr = `CASE
			WHEN lower(name) LIKE ? THEN 1
			WHEN lower(name) LIKE ? THEN 2
			ELSE 3
		END`
		scoreArgs = append(scoreArgs, markerLike, sessionLike)
	} else if marker != "" {
		markerLike := "%" + marker + "%"
		scoreExpr = `CASE WHEN lower(name) LIKE ? THEN 1 ELSE 2 END`
		scoreArgs = append(scoreArgs, markerLike)
	} else {
		sessionLike := "%" + sessionName + "%"
		scoreExpr = `CASE WHEN lower(name) LIKE ? THEN 1 ELSE 2 END`
		scoreArgs = append(scoreArgs, sessionLike)
	}

	query := `SELECT id, name, description, steps_json, enabled, webhook_url, parameters, created_at, updated_at
		FROM ops_runbooks
		WHERE ` + where + `
		ORDER BY ` + scoreExpr + `, name ASC
		LIMIT 5`

	allArgs := make([]any, 0, len(args)+len(scoreArgs))
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, scoreArgs...)

	rows, err := s.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("suggest runbooks for marker: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runbooks := make([]OpsRunbook, 0, 5)
	for rows.Next() {
		var (
			item       OpsRunbook
			stepsJSON  string
			paramsJSON string
			enabled    int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&stepsJSON,
			&enabled,
			&item.WebhookURL,
			&paramsJSON,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(stepsJSON), &item.Steps); err != nil {
			item.Steps = []OpsRunbookStep{}
		}
		if err := json.Unmarshal([]byte(paramsJSON), &item.Parameters); err != nil || item.Parameters == nil {
			item.Parameters = []RunbookParameter{}
		}
		runbooks = append(runbooks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runbooks, nil
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
