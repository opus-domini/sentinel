# Architecture

Sentinel is a single Go binary with embedded frontend assets and a local SQLite data plane.

## High-Level Components

- `cmd/sentinel`: CLI, server bootstrap, service/update/recovery commands.
- `internal/httpui`: SPA delivery and WebSocket endpoints.
- `internal/api`: authenticated HTTP API for tmux, recovery, operations, and metadata.
- `internal/tmux`: tmux command adapter and behavior patches.
- `internal/watchtower`: activity collector and unread projection engine.
- `internal/recovery`: periodic snapshot and restore orchestration.
- `internal/store`: SQLite schema and persistence (sessions metadata, watchtower, recovery, guardrails).
- `client`: React/Vite frontend with file-based routing (TanStack Router), optimistic UX, and event-driven sync. Routes: `/tmux`, `/services`, `/runbooks`, `/alerts`, `/timeline`, `/metrics`.

## Runtime Flow

1. Server starts and loads config.
2. Security guard applies token/origin policy.
3. Watchtower and recovery services start (if enabled).
4. Frontend connects:
   - REST for initial snapshot (`/api/...`)
   - WebSocket for realtime updates (`/ws/events`)
   - PTY stream (`/ws/tmux`)
5. UI uses optimistic mutations and reconciles with events/patches.
6. UI routes provide dedicated pages: terminal workspace (`/tmux`), service management (`/services`), runbook execution (`/runbooks`), alert monitoring (`/alerts`), operations timeline (`/timeline`), and system metrics (`/metrics`).

## Data Model (Operational)

- Session metadata
- Watchtower projections:
  - session-level unread/activity state
  - window unread flags
  - pane revision/seen revision
  - journal revisions (`global_rev`) for delta sync
  - timeline events
  - live presence
- Recovery snapshots and restore jobs
- Guardrail rules and audit logs

## Event-Driven UX Strategy

Primary path is WS events:

- `tmux.sessions.updated`
- `tmux.inspector.updated`
- `tmux.activity.updated`
- `tmux.timeline.updated`
- `ops.overview.updated`
- `ops.services.updated`
- `ops.alerts.updated`
- `ops.timeline.updated`
- `ops.job.updated`
- `recovery.overview.updated`
- `recovery.job.updated`

Fallback HTTP polling is used only when events channel is disconnected.

## Design Principles

- Single-binary deployment.
- No cloud relay by default.
- Optimistic frontend interactions for responsiveness.
- Server remains source of truth through projections and event patches.
- Feature gates via config for watchtower and recovery subsystems.
- Dedicated pages per concern: each operational feature has its own route, sidebar, and help dialog for focused workflows.
