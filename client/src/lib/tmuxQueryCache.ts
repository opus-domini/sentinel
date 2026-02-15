import type { PaneInfo, WindowInfo } from '@/types'

export const TMUX_SESSIONS_QUERY_KEY = ['tmux', 'sessions'] as const

export type TmuxInspectorSnapshot = {
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
}

export function tmuxInspectorQueryKey(
  session: string,
): readonly ['tmux', 'inspector', string] {
  return ['tmux', 'inspector', session.trim()] as const
}

export function shouldCacheActiveInspectorSnapshot(
  activeSession: string,
  windows: Array<WindowInfo>,
  panes: Array<PaneInfo>,
): boolean {
  const active = activeSession.trim()
  if (active === '') return false

  const windowSession =
    windows.find((item) => item.session.trim() !== '')?.session.trim() ?? ''
  if (windowSession !== '') {
    return windowSession === active
  }

  const paneSession =
    panes.find((item) => item.session.trim() !== '')?.session.trim() ?? ''
  if (paneSession !== '') {
    return paneSession === active
  }

  return false
}
