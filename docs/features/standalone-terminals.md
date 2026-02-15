# Standalone Terminals

Besides tmux workspaces, Sentinel also supports standalone shell terminals.

## Purpose

Use standalone terminals when you need ephemeral tabs not tied to tmux state.

- Route: `/terminals`
- WebSocket PTY: `/ws/terminals?terminal=<name>`

## Behavior

- Terminal tabs are frontend-managed and named `terminal-<n>`.
- Each tab attaches to an independent shell PTY.
- Closing a tab closes its WS stream.

## System Terminal Inspection

Sentinel can also inspect host TTYs:

- `GET /api/terminals` lists system terminals.
- `GET /api/terminals/system/{tty...}` lists processes for one TTY.
- `DELETE /api/terminals/{terminal}` closes tracked terminal session.

Frontend refreshes system terminal list periodically in the terminals route.

## Security

All terminal endpoints follow the same auth/origin guard as tmux APIs and WS endpoints.
