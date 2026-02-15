# Tmux Workspace

Sentinel models tmux as:

- Session
- Window
- Pane

The UI and API are built to keep this hierarchy consistent with realtime updates.

## Core Capabilities

- List/create/rename/kill sessions.
- List/select/create/kill windows.
- List/select/split/kill panes.
- Attach to any session over WebSocket PTY stream.
- Rename window and pane labels.
- Session icon metadata.

## Realtime Interaction

- `/ws/tmux?session=<name>` streams the active tmux PTY.
- `/ws/events` carries projection updates for lists, unread state, and recovery status.

Mouse stability in browser terminals is enforced server-side by tmux binding patches and mouse-mode enablement.

## Optimistic UX

Frontend actions assume success first and reconcile with backend events.

Examples:

- Session create/kill updates UI immediately.
- Window create/kill/select is optimistic.
- Pane split/select/kill is optimistic.

If backend rejects the action, UI is corrected by subsequent API/event reconciliation.

## Default Naming Rules

When creating entities without custom names:

- New window: `win-<sequence>` where sequence is monotonic per session.
- New pane title: `pan-<pane-id-suffix>`.

This avoids repeated ambiguous names after tmux index reuse.

## Unread and Activity Semantics

Watchtower tracks revisions per pane and seen revisions per focus scope.

- Pane receives new output -> pane can become unread.
- Window is considered unread when any pane in it is unread.
- Session summary aggregates unread windows/panes.

Seen operations happen via WS events channel (`type: "seen"`) and emit patch updates immediately.
