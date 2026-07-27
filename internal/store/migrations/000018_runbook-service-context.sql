ALTER TABLE ops_runbooks
ADD COLUMN target_service TEXT NOT NULL DEFAULT '';

UPDATE ops_runbooks
SET target_service = 'sentinel'
WHERE id = 'ops.service.recover';

UPDATE ops_runbooks
SET target_service = 'sentinel-updater'
WHERE id = 'ops.autoupdate.verify';

CREATE UNIQUE INDEX idx_ops_runbooks_target_service
ON ops_runbooks (target_service)
WHERE target_service <> '';

ALTER TABLE ops_runbook_runs
ADD COLUMN source TEXT NOT NULL DEFAULT '';

ALTER TABLE ops_runbook_runs
ADD COLUMN target_kind TEXT NOT NULL DEFAULT '';

ALTER TABLE ops_runbook_runs
ADD COLUMN target_name TEXT NOT NULL DEFAULT '';
