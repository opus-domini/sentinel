# Architecture

Sentinel is a single Go binary with embedded frontend assets and a local SQLite data plane.

## High-Level Components

- `cmd/sentinel`: thin entrypoint — wires args and the process exit code.
- `internal/cli`: command parsing, help, completion, and formatted output.
- `internal/server`: HTTP server bootstrap, background tickers, pinned-session restore.
- `internal/ui`: SPA delivery, embedded frontend assets and WebSocket endpoints.
- `internal/api`: authenticated HTTP API for tmux, operations, and metadata.
- `internal/tmux`: tmux command adapter and behavior patches.
- `internal/watchtower`: activity collector and unread projection engine.
- `internal/store`: SQLite schema and persistence (sessions metadata, watchtower activity, runbooks, schedules, and services).
- `internal/notify`: webhook delivery with retry/backoff for runbook and health report notifications.
- `internal/report`: scheduled health report generation and webhook dispatch.
- `internal/runbook`: runbook definition parsing, step execution (run/script/approval), shell validation, and webhook dispatch.
- `internal/scheduler`: cron-based job scheduling and execution engine.
- `internal/term`: terminal abstraction and PTY lifecycle management.
- `internal/updater`: binary self-update checks and apply logic.
- `internal/validate`: shared input validators (session names, cron expressions, timezones).
- `internal/config`: TOML configuration loading with environment variable overrides (`SENTINEL_*`).
- `internal/security`: bearer token authentication and CORS origin validation.
- `internal/daemon`: systemd/launchd service install and lifecycle management.
- `frontend`: React/Vite frontend with file-based routing (TanStack Router), optimistic UX, and event-driven sync. Routes: `/tmux`, `/services`, `/ops`, `/runbooks`, `/metrics`.

## Runtime Flow

1. Server starts and loads config.
2. Security guard applies token/origin policy.
3. Watchtower and operations services start.
4. Frontend connects:
   - REST for initial snapshot (`/api/...`)
   - WebSocket for realtime updates (`/ws/events`)
   - PTY stream (`/ws/tmux`)
5. UI uses optimistic mutations and reconciles with events/patches.
6. UI routes provide dedicated pages: terminal workspace (`/tmux`), service management (`/services`), operations dashboard (`/ops`), runbook execution (`/runbooks`), and system metrics (`/metrics`).

## Data Model (Operational)

- Session metadata
- Watchtower projections:
  - session-level unread/activity state
  - window unread flags
  - pane revision/seen revision
  - journal revisions (`global_rev`) for delta sync
  - live presence
- Ops runbooks, runs, schedules, and parameters
- Session directory frequency tracking
- Session users registry (`session_users`)
- Tmux launchers with user targeting (`tmux_launchers`)
- Session presets (`session_presets`)

## Event-Driven UX Strategy

Primary path is WS events:

- `tmux.sessions.updated`
- `tmux.inspector.updated`
- `tmux.activity.updated`
- `ops.overview.updated`
- `ops.services.updated`
- `ops.metrics.updated`
- `ops.schedule.updated`
- `ops.job.updated`

Fallback HTTP polling is used only when events channel is disconnected.

## Design Principles

- Single-binary deployment.
- No cloud relay by default.
- Optimistic frontend interactions for responsiveness.
- Server remains source of truth through projections and event patches.
- Dedicated pages per concern: each operational feature has its own route, sidebar, and help dialog for focused workflows.
