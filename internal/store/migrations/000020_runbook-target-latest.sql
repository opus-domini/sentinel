CREATE INDEX idx_ops_runbook_runs_target_latest
ON ops_runbook_runs (target_kind, target_name, created_at DESC, id DESC);
