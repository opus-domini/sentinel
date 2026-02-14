import { describe, expect, it } from 'vitest'

import {
  addPendingPaneClose,
  addPendingWindowClose,
  addPendingWindowCreate,
  clearPendingPaneClosesForSession,
  clearPendingWindowClosesForSession,
  clearPendingWindowCreatesForSession,
  clearPendingWindowPaneFloor,
  clearPendingWindowPaneFloorsForSession,
  mergePendingInspectorSnapshot,
  removePendingPaneClose,
  removePendingWindowClose,
  removePendingWindowCreate,
  setPendingWindowPaneFloor,
} from './tmuxInspectorOptimistic'
import type { PaneInfo, WindowInfo } from '@/types'

function buildWindow(session: string, index: number, panes: number): WindowInfo {
  return {
    session,
    index,
    name: `w-${index}`,
    active: false,
    panes,
    unreadPanes: 0,
    hasUnread: false,
    rev: 1,
  }
}

function buildPane(
  session: string,
  windowIndex: number,
  paneIndex: number,
  paneId: string,
): PaneInfo {
  return {
    session,
    windowIndex,
    paneIndex,
    paneId,
    title: paneId,
    active: false,
    tty: '',
    hasUnread: false,
    revision: 1,
    seenRevision: 1,
  }
}

describe('pending optimistic inspector maps', () => {
  it('tracks window create/close and pane close lifecycles per session', () => {
    const pendingCreates = new Map<string, Set<number>>()
    const pendingCloses = new Map<string, Set<number>>()
    const pendingPaneCloses = new Map<string, Set<string>>()
    const pendingFloors = new Map<string, Map<number, number>>()

    addPendingWindowCreate(pendingCreates, 'alpha', 2)
    addPendingWindowCreate(pendingCreates, 'alpha', 3)
    addPendingWindowClose(pendingCloses, 'alpha', 1)
    addPendingPaneClose(pendingPaneCloses, 'alpha', '%5')
    setPendingWindowPaneFloor(pendingFloors, 'alpha', 1, 3)
    setPendingWindowPaneFloor(pendingFloors, 'alpha', 1, 2)

    expect(Array.from(pendingCreates.get('alpha') ?? [])).toEqual([2, 3])
    expect(Array.from(pendingCloses.get('alpha') ?? [])).toEqual([1])
    expect(Array.from(pendingPaneCloses.get('alpha') ?? [])).toEqual(['%5'])
    expect(pendingFloors.get('alpha')?.get(1)).toBe(3)

    removePendingWindowCreate(pendingCreates, 'alpha', 2)
    removePendingWindowClose(pendingCloses, 'alpha', 1)
    removePendingPaneClose(pendingPaneCloses, 'alpha', '%5')
    clearPendingWindowPaneFloor(pendingFloors, 'alpha', 1)

    expect(Array.from(pendingCreates.get('alpha') ?? [])).toEqual([3])
    expect(pendingCloses.has('alpha')).toBe(false)
    expect(pendingPaneCloses.has('alpha')).toBe(false)
    expect(pendingFloors.has('alpha')).toBe(false)

    clearPendingWindowCreatesForSession(pendingCreates, 'alpha')
    clearPendingWindowClosesForSession(pendingCloses, 'alpha')
    clearPendingPaneClosesForSession(pendingPaneCloses, 'alpha')
    clearPendingWindowPaneFloorsForSession(pendingFloors, 'alpha')

    expect(pendingCreates.has('alpha')).toBe(false)
    expect(pendingCloses.has('alpha')).toBe(false)
    expect(pendingPaneCloses.has('alpha')).toBe(false)
    expect(pendingFloors.has('alpha')).toBe(false)
  })
})

describe('mergePendingInspectorSnapshot', () => {
  it('keeps pending-created window visible and confirms it when backend catches up', () => {
    const pendingCreates = new Map<string, Set<number>>([
      ['alpha', new Set([2])],
    ])
    const options = {
      pendingWindowCreates: pendingCreates,
      pendingWindowCloses: new Map<string, Set<number>>(),
      pendingPaneCloses: new Map<string, Set<string>>(),
      pendingWindowPaneFloors: new Map<string, Map<number, number>>(),
    }

    const initial = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 1)],
      [buildPane('alpha', 0, 0, '%1')],
      options,
    )
    expect(initial.windows.map((item) => item.index)).toEqual([0, 2])
    expect(initial.confirmedWindowCreates).toEqual([])

    const confirmed = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 1), buildWindow('alpha', 2, 1)],
      [buildPane('alpha', 0, 0, '%1'), buildPane('alpha', 2, 0, '%9')],
      options,
    )
    expect(confirmed.windows.map((item) => item.index)).toEqual([0, 2])
    expect(confirmed.confirmedWindowCreates).toEqual([2])
  })

  it('keeps pending-closed windows and panes hidden until backend converges', () => {
    const options = {
      pendingWindowCreates: new Map<string, Set<number>>(),
      pendingWindowCloses: new Map<string, Set<number>>([
        ['alpha', new Set([1])],
      ]),
      pendingPaneCloses: new Map<string, Set<string>>([
        ['alpha', new Set(['%2'])],
      ]),
      pendingWindowPaneFloors: new Map<string, Map<number, number>>(),
    }

    const stale = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 1), buildWindow('alpha', 1, 1)],
      [buildPane('alpha', 0, 0, '%1'), buildPane('alpha', 1, 0, '%2')],
      options,
    )
    expect(stale.windows.map((item) => item.index)).toEqual([0])
    expect(stale.panes.map((item) => item.paneId)).toEqual(['%1'])
    expect(stale.confirmedWindowCloses).toEqual([])
    expect(stale.confirmedPaneCloses).toEqual([])

    const converged = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 1)],
      [buildPane('alpha', 0, 0, '%1')],
      options,
    )
    expect(converged.confirmedWindowCloses).toEqual([1])
    expect(converged.confirmedPaneCloses).toEqual(['%2'])
  })

  it('preserves split-pane optimistic floor until backend reaches expected count', () => {
    const options = {
      pendingWindowCreates: new Map<string, Set<number>>(),
      pendingWindowCloses: new Map<string, Set<number>>(),
      pendingPaneCloses: new Map<string, Set<string>>(),
      pendingWindowPaneFloors: new Map<string, Map<number, number>>([
        ['alpha', new Map([[0, 3]])],
      ]),
    }

    const stale = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 2)],
      [buildPane('alpha', 0, 0, '%1'), buildPane('alpha', 0, 1, '%2')],
      options,
    )
    expect(stale.windows[0]?.panes).toBe(3)
    expect(stale.confirmedWindowPaneFloors).toEqual([])

    const converged = mergePendingInspectorSnapshot(
      'alpha',
      [buildWindow('alpha', 0, 3)],
      [
        buildPane('alpha', 0, 0, '%1'),
        buildPane('alpha', 0, 1, '%2'),
        buildPane('alpha', 0, 2, '%3'),
      ],
      options,
    )
    expect(converged.windows[0]?.panes).toBe(3)
    expect(converged.confirmedWindowPaneFloors).toEqual([0])
  })
})
