package store

import "context"

const opsRunbookRunSelect = `
	id, runbook_id, runbook_name, status, total_steps, completed_steps,
	current_step, error, step_results, parameters_used, source, target_kind,
	target_name, definition_snapshot, created_at, started_at, finished_at`

// ListOpsRunbookActiveRuns returns every execution that can still change
// state. Now applies its own display cap after approvals and active runs have
// been classified, so this query deliberately has no arbitrary global limit.
func (s *Store) ListOpsRunbookActiveRuns(ctx context.Context) ([]OpsRunbookRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT`+opsRunbookRunSelect+`
	FROM ops_runbook_runs
	WHERE status IN (?, ?, ?)
	ORDER BY created_at DESC, id DESC`,
		OpsRunbookStatusQueued,
		OpsRunbookStatusRunning,
		OpsRunbookStatusWaitingApproval,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]OpsRunbookRun, 0, 8)
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

// ListOpsRunbookLatestTerminalRuns returns the most recent terminal execution
// for each runbook. A newer active execution intentionally does not hide the
// latest terminal outcome.
func (s *Store) ListOpsRunbookLatestTerminalRuns(ctx context.Context) ([]OpsRunbookRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT`+opsRunbookRunSelect+`
	FROM (
		SELECT`+opsRunbookRunSelect+`,
			ROW_NUMBER() OVER (
				PARTITION BY runbook_id
				ORDER BY created_at DESC, id DESC
			) AS terminal_rank
		FROM ops_runbook_runs
		WHERE status IN (?, ?)
	)
	WHERE terminal_rank = 1
	ORDER BY created_at DESC, id DESC`,
		OpsRunbookStatusSucceeded,
		OpsRunbookStatusFailed,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]OpsRunbookRun, 0, 8)
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
