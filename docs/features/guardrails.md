# Guardrails

Guardrails are policy rules evaluated before sensitive tmux operations.

## Rule Model

Each rule has:

- `id`, `name`
- `scope`: `action` (all rules are normalized to action scope)
- `pattern`: regex matched against the action string
- `mode`: `warn`, `confirm`, `block`
- `severity`: `info`, `warn`, `error`
- `enabled`, `priority`

Rules are ordered by priority and strongest matching decision wins (`block > confirm > warn`).

## Default Seed Rules

- `action.session.kill.confirm` — Confirm session kill (mode: confirm, priority 10)
- `action.pane.kill.warn` — Warn on pane kill (mode: warn, priority 20)

## Enforcement

Guardrails is the sole confirmation authority for destructive tmux actions. Every destructive tmux operation (`session.create`, `session.kill`, `window.create`, `window.kill`, `pane.kill`, `pane.split`) is routed through `enforceGuardrail()` before execution.

- `block` — API returns `409 GUARDRAIL_BLOCKED`; action is rejected
- `confirm` without confirmation — `428 GUARDRAIL_CONFIRM_REQUIRED`; action is rejected until confirmed
- `confirm` with confirmation — action proceeds with audit override
- `warn` — action proceeds; decision is written to audit

Confirmation can be provided by:

- Header: `X-Sentinel-Guardrail-Confirm: true`
- Query: `?confirm=true`

## Realtime UX

When blocked/confirm-required, backend emits:

- `tmux.guardrail.blocked`

Frontend consumes this event to display immediate user feedback.

## API Endpoints

- `GET /api/ops/guardrails/rules` — list all rules
- `POST /api/ops/guardrails/rules` — create a new rule
- `PATCH /api/ops/guardrails/rules/{rule}` — update a rule
- `DELETE /api/ops/guardrails/rules/{rule}` — delete a rule
- `GET /api/ops/guardrails/audit` — list audit entries
- `POST /api/ops/guardrails/evaluate` — dry-run evaluate an action against policy

## Frontend

GuardrailsDialog is a tmux-scoped dialog opened from the tmux header (ShieldAlert icon), not a standalone route.

## Audit Trail

Every matched guardrail decision (block, confirm, warn) is written to `guardrail_audit` with action context, override flag, reason, and metadata.
