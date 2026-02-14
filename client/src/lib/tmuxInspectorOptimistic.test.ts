import { describe, expect, it } from 'vitest'

import {
  addPendingWindowCreate,
  clearPendingWindowCreatesForSession,
  mergePendingWindowCreates,
  removePendingWindowCreate,
} from './tmuxInspectorOptimistic'
import type { WindowInfo } from '@/types'

function buildWindow(session: string, index: number): WindowInfo {
  return {
    session,
    index,
    name: `w-${index}`,
    active: false,
    panes: 1,
    unreadPanes: 0,
    hasUnread: false,
    rev: 1,
  }
}

describe('pending window create tracking', () => {
  it('adds and removes pending indexes per session', () => {
    const pending = new Map<string, Set<number>>()

    addPendingWindowCreate(pending, 'alpha', 3)
    addPendingWindowCreate(pending, 'alpha', 4)
    addPendingWindowCreate(pending, 'beta', 1)

    expect(Array.from(pending.get('alpha') ?? [])).toEqual([3, 4])
    expect(Array.from(pending.get('beta') ?? [])).toEqual([1])

    removePendingWindowCreate(pending, 'alpha', 3)
    expect(Array.from(pending.get('alpha') ?? [])).toEqual([4])

    removePendingWindowCreate(pending, 'alpha', 4)
    expect(pending.has('alpha')).toBe(false)
  })

  it('clears a full session namespace', () => {
    const pending = new Map<string, Set<number>>([
      ['alpha', new Set([1, 2])],
      ['beta', new Set([3])],
    ])

    clearPendingWindowCreatesForSession(pending, 'alpha')

    expect(pending.has('alpha')).toBe(false)
    expect(Array.from(pending.get('beta') ?? [])).toEqual([3])
  })
})

describe('mergePendingWindowCreates', () => {
  it('keeps optimistic windows visible until backend confirms them', () => {
    const pending = new Map<string, Set<number>>([
      ['alpha', new Set([3, 4])],
    ])
    const backend = [buildWindow('alpha', 0), buildWindow('alpha', 1)]

    const merged = mergePendingWindowCreates('alpha', backend, pending)

    expect(merged.windows.map((item) => item.index)).toEqual([0, 1, 3, 4])
    expect(merged.windows[2].name).toBe('new')
    expect(merged.confirmedPendingIndexes).toEqual([])
  })

  it('marks pending windows as confirmed when backend already includes them', () => {
    const pending = new Map<string, Set<number>>([
      ['alpha', new Set([2, 3])],
    ])
    const backend = [
      buildWindow('alpha', 0),
      buildWindow('alpha', 2),
      buildWindow('alpha', 3),
    ]

    const merged = mergePendingWindowCreates('alpha', backend, pending)

    expect(merged.windows.map((item) => item.index)).toEqual([0, 2, 3])
    expect(merged.confirmedPendingIndexes).toEqual([2, 3])
  })

  it('does not mix pending windows from other sessions', () => {
    const pending = new Map<string, Set<number>>([
      ['beta', new Set([5])],
    ])
    const backend = [buildWindow('alpha', 0)]

    const merged = mergePendingWindowCreates('alpha', backend, pending)

    expect(merged.windows.map((item) => item.index)).toEqual([0])
    expect(merged.confirmedPendingIndexes).toEqual([])
  })
})
