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
