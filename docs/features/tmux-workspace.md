# Tmux Workspace

<img src="assets/images/desktop-tmux-mission-control.png" alt="Tmux attached to an isolated flight-control session, with two real panes showing fictitious orbital telemetry, recovery evidence, and guidance context" />

The Orbital Station terminal output is fictitious showcase data running in
disposable tmux sessions; Sentinel has no satellite-specific behavior.

Sentinel models tmux as:

- Session
- Window
- Pane

The UI and API are built to keep this hierarchy consistent with realtime updates.

## Core Capabilities

- List/create/rename/kill sessions.
- Create sessions from reusable session launchers in the sidebar `+` menu.
- List/select/create/kill windows.
- Create windows from reusable launchers in the window-strip `+` menu.
- List/select/split/kill panes.
- Attach to any session over WebSocket PTY stream.
- Rename window and pane labels.
- Session icon metadata.
- Frequent directories endpoint (`GET /api/tmux/frequent-dirs`) powers quick-pick suggestions in the session creation dialog.

## Realtime Interaction

- `/ws/tmux?session=<name>` streams the active tmux PTY.
- `/ws/events` carries projection updates for lists, unread state, and pane activity.

Mouse stability in browser terminals is enforced server-side by tmux binding patches and mouse-mode enablement.

## Session Lifecycle

Sessions created by people in the SPA, including sessions started from pinned
presets and launchers, are always persistent. The web interface has no lifetime
selector and does not offer keep, extend, or lifecycle-close actions.

The MCP integration can create ephemeral sessions. Sentinel identifies those
sessions with a static hourglass in the sessions sidebar and in open session
tabs, including the compact mobile layout. Its tooltip describes the current
state, absolute deadline, and remaining time at minute/hour granularity. The
marker does not animate or run a local countdown; lifecycle events refresh the
server projection.

An hourglass can report `active`, a cleanup grace period, or cleanup blocked by
an attached client. The blocked state means a human or external tmux client is
still attached after Sentinel drained its MCP attachments, so automatic cleanup
will not kill the session. Sessions without the marker are persistent or
otherwise unmanaged by the MCP lifecycle.

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

When creating a session with a name that already exists, the server auto-suffixes the name with `-1`, `-2`, ... up to `-99` to resolve the collision. The response `name` field may differ from the requested name.

## Pinned Sessions and Launchers

Sentinel uses the same split-button pattern for reusable tmux entrypoints at both workspace levels:

- The window-strip `+` button opens a blank window immediately. Its dropdown exposes the last used launcher, saved window launchers, and `Manage launchers...`.
- The sessions sidebar `+` button opens a blank session immediately. Its dropdown exposes `New blank session`, `Last used`, saved session launchers, and `Manage session launchers...`.

Pinned sessions are separate from launchers. They are backed by `/api/tmux/session-presets`, render in the `Pinned` panel, and are restored on Sentinel startup so important workspaces come back after an unexpected host restart.

Session launchers are backed by `/api/tmux/session-launchers`. They are reusable session creation presets stored in the session launcher manager. Saving a launcher does not start tmux. Launching one creates a new session from the launcher name seed, working directory, icon, and optional target user. Launching the same preset repeatedly creates numbered sessions (`api`, `api-1`, `api-2`, ...).

## OS Account Targeting

Sessions can be created under a selected operating-system account via the
`user` field. This changes the host identity of the Tmux process; it does not
create a Sentinel user, login, role, or tenant.

- Launchers support `userMode` (`session` or `fixed`) and `userValue` for
  per-launcher account targeting.
- The sidebar shows an account indicator when a session runs as someone other
  than the Sentinel process user.
- The `[multi_user]` configuration key has no enable flag. Its allowlist, root
  gate, known OS accounts, and switch method determine which targets are valid.

See [OS Account Targeting](/features/os-account-targeting.md) for the complete
host-execution and sudo/systemd requirements.

## Unread and Activity Semantics

The internal Tmux activity projection, implemented by the `watchtower` package,
tracks revisions per pane and seen revisions per focus scope. Watchtower is not
a separate product module or navigation destination.

- Pane receives new output -> pane can become unread.
- Window is considered unread when any pane in it is unread.
- Session summary aggregates unread windows/panes.
- Unread activity is indicated by the session icon colour in the sidebar (amber), not by a window badge.

Seen operations happen via WS events channel (`type: "seen"`) and emit patch updates immediately.

## Sidebar Density

The sidebar adapts to 3 tiers based on available width:

- Minimal (<=250px): icon + name, plus the lifecycle hourglass when applicable.
- Compact (<=300px): badges visible.
- Full (>300px): content preview visible.

Sidebar header actions use compact icon controls for add, help, and auth so the same pattern fits tmux and the operational sidebars on narrow layouts.
