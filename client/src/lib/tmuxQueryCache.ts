import type { PaneInfo, WindowInfo } from '@/types'

export const TMUX_SESSIONS_QUERY_KEY = ['tmux', 'sessions'] as const
export const TMUX_RECOVERY_OVERVIEW_QUERY_KEY = [
  'tmux',
  'recovery',
  'overview',
] as const

export type TmuxInspectorSnapshot = {
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
}

export function tmuxInspectorQueryKey(
  session: string,
): readonly ['tmux', 'inspector', string] {
  return ['tmux', 'inspector', session.trim()] as const
}

export function tmuxTimelineQueryKey(input: {
  session: string
  query: string
  severity: string
  eventType: string
  limit: number
}) {
  return [
    'tmux',
    'timeline',
    input.session.trim(),
    input.query.trim(),
    input.severity.trim().toLowerCase(),
    input.eventType.trim().toLowerCase(),
    Math.max(1, Math.trunc(input.limit)),
  ] as const
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
