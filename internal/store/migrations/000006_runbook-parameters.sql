-- 000006_runbook-parameters.sql: Add parameter support to runbooks.
-- Runbooks can define typed parameters that are substituted into step
-- commands before execution. Each run records the parameter values used.

ALTER TABLE ops_runbooks ADD COLUMN parameters TEXT NOT NULL DEFAULT '[]';
ALTER TABLE ops_runbook_runs ADD COLUMN parameters_used TEXT NOT NULL DEFAULT '{}';
