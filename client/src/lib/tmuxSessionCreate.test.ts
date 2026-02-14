import { describe, expect, it } from 'vitest'

import {
  buildOptimisticSession,
  mergePendingCreateSessions,
  upsertOptimisticAttachedSession,
} from './tmuxSessionCreate'

describe('buildOptimisticSession', () => {
  it('creates a fully attached placeholder session', () => {
    const at = '2026-02-14T12:00:00Z'
    const session = buildOptimisticSession('alpha', at)

    expect(session).toEqual({
      name: 'alpha',
      windows: 1,
      panes: 1,
      attached: 1,
      createdAt: at,
      activityAt: at,
      command: '',
      hash: '',
      lastContent: '',
      icon: '',
      unreadWindows: 0,
      unreadPanes: 0,
      rev: 0,
    })
  })
})

describe('upsertOptimisticAttachedSession', () => {
  it('appends placeholder when session does not exist', () => {
    const at = '2026-02-14T12:00:00Z'
    const next = upsertOptimisticAttachedSession([], 'beta', at)

    expect(next).toHaveLength(1)
    expect(next[0].name).toBe('beta')
    expect(next[0].attached).toBe(1)
    expect(next[0].activityAt).toBe(at)
  })

  it('marks existing session as attached and updates activity timestamp', () => {
    const at = '2026-02-14T12:01:00Z'
    const prev = [
      {
        name: 'beta',
        windows: 2,
        panes: 4,
        attached: 0,
        createdAt: '2026-02-14T11:59:00Z',
        activityAt: '2026-02-14T11:59:00Z',
        command: '',
        hash: 'abc123',
        lastContent: 'last',
        icon: 'terminal',
        unreadWindows: 0,
        unreadPanes: 0,
        rev: 1,
      },
    ]

    const next = upsertOptimisticAttachedSession(prev, 'beta', at)

    expect(next).toHaveLength(1)
    expect(next[0].attached).toBe(1)
    expect(next[0].activityAt).toBe(at)
    expect(next[0].hash).toBe('abc123')
  })

  it('does not duplicate sessions', () => {
    const at = '2026-02-14T12:02:00Z'
    const prev = [
      buildOptimisticSession('alpha', '2026-02-14T12:00:00Z'),
      buildOptimisticSession('beta', '2026-02-14T12:00:00Z'),
    ]

    const next = upsertOptimisticAttachedSession(prev, 'beta', at)

    expect(next).toHaveLength(2)
    expect(next.filter((item) => item.name === 'beta')).toHaveLength(1)
    expect(next[1].activityAt).toBe(at)
  })
})

describe('mergePendingCreateSessions', () => {
  it('keeps pending optimistic sessions until backend confirms them', () => {
    const backend = [buildOptimisticSession('stable', '2026-02-14T12:00:00Z')]
    const pending = new Map([
      ['new-a', '2026-02-14T12:01:00Z'],
      ['new-b', '2026-02-14T12:02:00Z'],
    ])

    const merged = mergePendingCreateSessions(backend, pending)

    expect(merged.sessions.map((item) => item.name)).toEqual([
      'stable',
      'new-a',
      'new-b',
    ])
    expect(merged.sessionNamesForSync).toEqual(['stable', 'new-a', 'new-b'])
    expect(merged.confirmedPendingNames).toEqual([])
  })

  it('marks pending sessions as confirmed when backend already contains them', () => {
    const backend = [
      buildOptimisticSession('stable', '2026-02-14T12:00:00Z'),
      buildOptimisticSession('new-a', '2026-02-14T12:03:00Z'),
    ]
    const pending = new Map([
      ['new-a', '2026-02-14T12:01:00Z'],
      ['new-b', '2026-02-14T12:02:00Z'],
    ])

    const merged = mergePendingCreateSessions(backend, pending)

    expect(merged.sessions.map((item) => item.name)).toEqual([
      'stable',
      'new-a',
      'new-b',
    ])
    expect(merged.confirmedPendingNames).toEqual(['new-a'])
  })
})
