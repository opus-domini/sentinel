# Guardrails

Guardrails are policy rules evaluated before sensitive tmux operations.

## Rule Model

Each rule has:

- `id`, `name`
- `scope`: `action` or `command`
- `pattern`: regex
- `mode`: `allow`, `warn`, `confirm`, `block`
- `severity`: `info`, `warn`, `error`
- `enabled`, `priority`

Rules are ordered by priority and strongest matching decision wins (`block > confirm > warn > allow`).

## Default Seed Rules

- `action.session.kill.confirm`
- `command.rm.root.block`

## Enforced Behaviors

- `block` -> API returns `409 GUARDRAIL_BLOCKED`
- `confirm` without confirmation -> `428 GUARDRAIL_CONFIRM_REQUIRED`
- `warn` -> allowed but audited

Confirmation can be provided by:

- Header: `X-Sentinel-Guardrail-Confirm: true`
- Query: `?confirm=true`

## Realtime UX

When blocked/confirm-required, backend emits:

- `tmux.guardrail.blocked`

Frontend consumes this event to display immediate user feedback.

## API Endpoints

- `GET /api/ops/guardrails/rules`
- `PATCH /api/ops/guardrails/rules/{rule}`
- `GET /api/ops/guardrails/audit`
- `POST /api/ops/guardrails/evaluate`

## Audit Trail

Every matched guardrail decision can be written to `guardrail_audit` with action context, override flag, reason, and metadata.
