import { describe, expect, it } from 'vitest'

import {
  TMUX_SESSIONS_QUERY_KEY,
  shouldCacheActiveInspectorSnapshot,
  tmuxInspectorQueryKey,
} from './tmuxQueryCache'
import type { PaneInfo, WindowInfo } from '@/types'

function windowFor(session: string): WindowInfo {
  return {
    session,
    index: 0,
    name: 'win-1',
    active: true,
    panes: 1,
  }
}

function paneFor(session: string): PaneInfo {
  return {
    session,
    windowIndex: 0,
    paneIndex: 0,
    paneId: '%0',
    title: 'pan-1',
    active: true,
    tty: '',
  }
}

describe('tmuxQueryCache', () => {
  it('builds stable query keys', () => {
    expect(TMUX_SESSIONS_QUERY_KEY).toEqual(['tmux', 'sessions'])
    expect(tmuxInspectorQueryKey(' alpha ')).toEqual([
      'tmux',
      'inspector',
      'alpha',
    ])
  })

  it('persists inspector snapshot only when current data belongs to active session', () => {
    expect(
      shouldCacheActiveInspectorSnapshot('alpha', [windowFor('alpha')], []),
    ).toBe(true)
    expect(
      shouldCacheActiveInspectorSnapshot('alpha', [windowFor('beta')], []),
    ).toBe(false)
  })

  it('falls back to pane session when windows are absent', () => {
    expect(
      shouldCacheActiveInspectorSnapshot('alpha', [], [paneFor('alpha')]),
    ).toBe(true)
    expect(
      shouldCacheActiveInspectorSnapshot('alpha', [], [paneFor('beta')]),
    ).toBe(false)
  })

  it('does not persist when session cannot be resolved', () => {
    expect(shouldCacheActiveInspectorSnapshot('alpha', [], [])).toBe(false)
    expect(
      shouldCacheActiveInspectorSnapshot('', [windowFor('alpha')], []),
    ).toBe(false)
  })
})
