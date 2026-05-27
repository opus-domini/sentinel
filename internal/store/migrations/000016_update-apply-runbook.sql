-- 000016_update-apply-runbook.sql: update built-in updater runbook command.

UPDATE ops_runbooks
SET steps_json = REPLACE(
    steps_json,
    'sentinel update apply --restart',
    'sentinel update apply'
)
WHERE id = 'ops.update.apply'
  AND steps_json LIKE '%sentinel update apply --restart%';
