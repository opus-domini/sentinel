const TMUX_NAME_MAX = 64
const TMUX_PANE_TITLE_MAX = 128

// Normalize to tmux-safe names: [A-Za-z0-9._-], max 64.
export function slugifyTmuxName(raw: string): string {
  return raw
    .replace(/\s+/g, '-')
    .replace(/[^A-Za-z0-9._-]/g, '')
    .slice(0, TMUX_NAME_MAX)
}

// Aligns with backend window validation: [A-Za-z0-9._- ] up to 64 chars.
export function sanitizeTmuxWindowName(raw: string): string {
  return raw.replace(/[^A-Za-z0-9._\- ]/g, '').slice(0, TMUX_NAME_MAX)
}

// Pane titles accept broader input; only cap length client-side.
export function sanitizeTmuxPaneTitle(raw: string): string {
  return raw.slice(0, TMUX_PANE_TITLE_MAX)
}
