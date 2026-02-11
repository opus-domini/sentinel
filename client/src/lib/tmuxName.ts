const TMUX_NAME_MAX = 64

// Normalize to tmux-safe names: [A-Za-z0-9._-], max 64.
export function slugifyTmuxName(raw: string): string {
  return raw
    .replace(/\s+/g, '-')
    .replace(/[^A-Za-z0-9._-]/g, '')
    .slice(0, TMUX_NAME_MAX)
}
