ALTER TABLE ops_runbook_runs
ADD COLUMN definition_snapshot TEXT NOT NULL DEFAULT '';

UPDATE ops_runbook_runs
SET status = 'failed',
    error = 'execution predates immutable receipt',
    finished_at = datetime('now')
WHERE definition_snapshot = ''
  AND status IN ('queued', 'running', 'waiting_approval');

CREATE UNIQUE INDEX idx_ops_runbook_runs_active_service_target
ON ops_runbook_runs (target_kind, target_name)
WHERE target_kind = 'service'
  AND status IN ('queued', 'running', 'waiting_approval');

DELETE FROM ops_schedules
WHERE runbook_id IN (
    SELECT id
    FROM ops_runbooks
    WHERE created_at = updated_at
      AND webhook_url = ''
      AND parameters = '[]'
      AND enabled = 1
      AND (
        (
          id = 'ops.service.recover'
          AND name = 'Service Recovery'
          AND description = 'Validate and recover the Sentinel service runtime.'
          AND target_service = 'sentinel'
          AND steps_json = '[{"type":"run","title":"Inspect service status","command":"sentinel service status"},{"type":"run","title":"Restart service","command":"sentinel service install --start=true"},{"type":"run","title":"Confirm healthy status","command":"service should be active"}]'
        )
        OR (
          id = 'ops.autoupdate.verify'
          AND name = 'Autoupdate Verification'
          AND description = 'Check updater configuration and latest release state.'
          AND target_service = 'sentinel-updater'
          AND steps_json = '[{"type":"run","title":"Check updater timer","command":"sentinel service autoupdate status"},{"type":"run","title":"Check release status","command":"sentinel update check"},{"type":"approval","title":"Review output","description":"Review versions and update policy before apply."}]'
        )
        OR (
          id = 'ops.update.apply'
          AND name = 'Apply Update'
          AND description = 'Check for updates, download and install the latest version, and restart the service.'
          AND target_service = ''
          AND steps_json = '[{"type":"run","title":"Check for updates","command":"sentinel update check"},{"type":"run","title":"Apply update and restart","command":"sentinel update apply"}]'
        )
      )
);

DELETE FROM ops_runbooks
WHERE created_at = updated_at
  AND webhook_url = ''
  AND parameters = '[]'
  AND enabled = 1
  AND (
    (
      id = 'ops.service.recover'
      AND name = 'Service Recovery'
      AND description = 'Validate and recover the Sentinel service runtime.'
      AND target_service = 'sentinel'
      AND steps_json = '[{"type":"run","title":"Inspect service status","command":"sentinel service status"},{"type":"run","title":"Restart service","command":"sentinel service install --start=true"},{"type":"run","title":"Confirm healthy status","command":"service should be active"}]'
    )
    OR (
      id = 'ops.autoupdate.verify'
      AND name = 'Autoupdate Verification'
      AND description = 'Check updater configuration and latest release state.'
      AND target_service = 'sentinel-updater'
      AND steps_json = '[{"type":"run","title":"Check updater timer","command":"sentinel service autoupdate status"},{"type":"run","title":"Check release status","command":"sentinel update check"},{"type":"approval","title":"Review output","description":"Review versions and update policy before apply."}]'
    )
    OR (
      id = 'ops.update.apply'
      AND name = 'Apply Update'
      AND description = 'Check for updates, download and install the latest version, and restart the service.'
      AND target_service = ''
      AND steps_json = '[{"type":"run","title":"Check for updates","command":"sentinel update check"},{"type":"run","title":"Apply update and restart","command":"sentinel update apply"}]'
    )
  );
