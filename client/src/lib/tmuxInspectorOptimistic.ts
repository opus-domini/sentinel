import type { PaneInfo, WindowInfo } from '@/types'

export type PendingWindowIndexMap = Map<string, Set<number>>
export type PendingPaneIDMap = Map<string, Set<string>>
export type PendingWindowPaneFloorMap = Map<string, Map<number, number>>

function normalizeSession(session: string): string {
  return session.trim()
}

function normalizeWindowIndex(windowIndex: number): number | null {
  if (!Number.isFinite(windowIndex) || windowIndex < 0) {
    return null
  }
  return Math.trunc(windowIndex)
}

function upsertWindowIndex(
  target: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  const name = normalizeSession(session)
  const index = normalizeWindowIndex(windowIndex)
  if (name === '' || index === null) return
  const existing = target.get(name)
  if (existing) {
    existing.add(index)
    return
  }
  target.set(name, new Set([index]))
}

function deleteWindowIndex(
  target: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  const name = normalizeSession(session)
  const index = normalizeWindowIndex(windowIndex)
  if (name === '' || index === null) return
  const existing = target.get(name)
  if (!existing) return
  existing.delete(index)
  if (existing.size === 0) {
    target.delete(name)
  }
}

export function addPendingWindowCreate(
  pendingCreates: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  upsertWindowIndex(pendingCreates, session, windowIndex)
}

export function removePendingWindowCreate(
  pendingCreates: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  deleteWindowIndex(pendingCreates, session, windowIndex)
}

export function clearPendingWindowCreatesForSession(
  pendingCreates: PendingWindowIndexMap,
  session: string,
): void {
  const name = normalizeSession(session)
  if (name === '') return
  pendingCreates.delete(name)
}

export function addPendingWindowClose(
  pendingCloses: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  upsertWindowIndex(pendingCloses, session, windowIndex)
}

export function removePendingWindowClose(
  pendingCloses: PendingWindowIndexMap,
  session: string,
  windowIndex: number,
): void {
  deleteWindowIndex(pendingCloses, session, windowIndex)
}

export function clearPendingWindowClosesForSession(
  pendingCloses: PendingWindowIndexMap,
  session: string,
): void {
  const name = normalizeSession(session)
  if (name === '') return
  pendingCloses.delete(name)
}

export function addPendingPaneClose(
  pendingPaneCloses: PendingPaneIDMap,
  session: string,
  paneID: string,
): void {
  const name = normalizeSession(session)
  const id = paneID.trim()
  if (name === '' || id === '') return
  const existing = pendingPaneCloses.get(name)
  if (existing) {
    existing.add(id)
    return
  }
  pendingPaneCloses.set(name, new Set([id]))
}

export function removePendingPaneClose(
  pendingPaneCloses: PendingPaneIDMap,
  session: string,
  paneID: string,
): void {
  const name = normalizeSession(session)
  const id = paneID.trim()
  if (name === '' || id === '') return
  const existing = pendingPaneCloses.get(name)
  if (!existing) return
  existing.delete(id)
  if (existing.size === 0) {
    pendingPaneCloses.delete(name)
  }
}

export function clearPendingPaneClosesForSession(
  pendingPaneCloses: PendingPaneIDMap,
  session: string,
): void {
  const name = normalizeSession(session)
  if (name === '') return
  pendingPaneCloses.delete(name)
}

export function setPendingWindowPaneFloor(
  pendingPaneFloors: PendingWindowPaneFloorMap,
  session: string,
  windowIndex: number,
  paneFloor: number,
): void {
  const name = normalizeSession(session)
  const index = normalizeWindowIndex(windowIndex)
  if (
    name === '' ||
    index === null ||
    !Number.isFinite(paneFloor) ||
    paneFloor < 0
  ) {
    return
  }
  const floor = Math.trunc(paneFloor)
  const existing = pendingPaneFloors.get(name)
  if (!existing) {
    pendingPaneFloors.set(name, new Map([[index, floor]]))
    return
  }
  existing.set(index, Math.max(floor, existing.get(index) ?? 0))
}

export function clearPendingWindowPaneFloor(
  pendingPaneFloors: PendingWindowPaneFloorMap,
  session: string,
  windowIndex: number,
): void {
  const name = normalizeSession(session)
  const index = normalizeWindowIndex(windowIndex)
  if (name === '' || index === null) return
  const existing = pendingPaneFloors.get(name)
  if (!existing) return
  existing.delete(index)
  if (existing.size === 0) {
    pendingPaneFloors.delete(name)
  }
}

export function clearPendingWindowPaneFloorsForSession(
  pendingPaneFloors: PendingWindowPaneFloorMap,
  session: string,
): void {
  const name = normalizeSession(session)
  if (name === '') return
  pendingPaneFloors.delete(name)
}

export type MergePendingInspectorOptions = {
  pendingWindowCreates: ReadonlyMap<string, ReadonlySet<number>>
  pendingWindowCloses: ReadonlyMap<string, ReadonlySet<number>>
  pendingPaneCloses: ReadonlyMap<string, ReadonlySet<string>>
  pendingWindowPaneFloors: ReadonlyMap<string, ReadonlyMap<number, number>>
}

export type MergePendingInspectorResult = {
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
  confirmedWindowCreates: Array<number>
  confirmedWindowCloses: Array<number>
  confirmedPaneCloses: Array<string>
  confirmedWindowPaneFloors: Array<number>
}

export function mergePendingInspectorSnapshot(
  session: string,
  windows: Array<WindowInfo>,
  panes: Array<PaneInfo>,
  options: MergePendingInspectorOptions,
): MergePendingInspectorResult {
  const name = normalizeSession(session)
  if (name === '') {
    return {
      windows,
      panes,
      confirmedWindowCreates: [],
      confirmedWindowCloses: [],
      confirmedPaneCloses: [],
      confirmedWindowPaneFloors: [],
    }
  }

  const pendingWindowCreateSet = options.pendingWindowCreates.get(name)
  const pendingWindowCloseSet = options.pendingWindowCloses.get(name)
  const pendingPaneCloseSet = options.pendingPaneCloses.get(name)
  const pendingWindowPaneFloorMap = options.pendingWindowPaneFloors.get(name)

  const backendWindowIndexes = new Set(
    windows
      .map((windowInfo) => normalizeWindowIndex(windowInfo.index))
      .filter((index): index is number => index !== null),
  )
  const backendPaneIDs = new Set(
    panes
      .map((paneInfo) => paneInfo.paneId.trim())
      .filter((paneID) => paneID !== ''),
  )

  const blockedWindowIndexes = new Set<number>()
  const confirmedWindowCloses: Array<number> = []
  for (const rawIndex of pendingWindowCloseSet ?? []) {
    const index = normalizeWindowIndex(rawIndex)
    if (index === null) continue
    blockedWindowIndexes.add(index)
    if (!backendWindowIndexes.has(index)) {
      confirmedWindowCloses.push(index)
    }
  }

  const optimisticCreatedWindowIndexes = new Set<number>()
  const confirmedWindowCreates: Array<number> = []
  const mergedWindows = windows.filter(
    (windowInfo) => !blockedWindowIndexes.has(windowInfo.index),
  )
  const seenWindowIndexes = new Set(
    mergedWindows.map((windowInfo) => normalizeWindowIndex(windowInfo.index)),
  )
  for (const rawIndex of pendingWindowCreateSet ?? []) {
    const index = normalizeWindowIndex(rawIndex)
    if (index === null || blockedWindowIndexes.has(index)) continue
    if (backendWindowIndexes.has(index)) {
      confirmedWindowCreates.push(index)
      continue
    }
    if (seenWindowIndexes.has(index)) {
      continue
    }
    seenWindowIndexes.add(index)
    optimisticCreatedWindowIndexes.add(index)
    mergedWindows.push({
      session: name,
      index,
      name: 'new',
      active: false,
      panes: 1,
      unreadPanes: 0,
      hasUnread: false,
      rev: 0,
    })
  }
  if (
    blockedWindowIndexes.size > 0 ||
    optimisticCreatedWindowIndexes.size > 0
  ) {
    mergedWindows.sort((left, right) => left.index - right.index)
  }

  const blockedPaneIDs = new Set<string>()
  const confirmedPaneCloses: Array<string> = []
  for (const rawPaneID of pendingPaneCloseSet ?? []) {
    const paneID = rawPaneID.trim()
    if (paneID === '') continue
    blockedPaneIDs.add(paneID)
    if (!backendPaneIDs.has(paneID)) {
      confirmedPaneCloses.push(paneID)
    }
  }

  const visibleWindowIndexes = new Set(
    mergedWindows.map((windowInfo) => normalizeWindowIndex(windowInfo.index)),
  )
  const mergedPanes = panes.filter((paneInfo) => {
    const paneID = paneInfo.paneId.trim()
    if (paneID === '' || blockedPaneIDs.has(paneID)) {
      return false
    }
    const windowIndex = normalizeWindowIndex(paneInfo.windowIndex)
    if (windowIndex === null || blockedWindowIndexes.has(windowIndex)) {
      return false
    }
    return visibleWindowIndexes.has(windowIndex)
  })

  const paneCountByWindow = new Map<number, number>()
  for (const paneInfo of mergedPanes) {
    const windowIndex = normalizeWindowIndex(paneInfo.windowIndex)
    if (windowIndex === null) continue
    paneCountByWindow.set(windowIndex, (paneCountByWindow.get(windowIndex) ?? 0) + 1)
  }

  const confirmedWindowPaneFloors: Array<number> = []
  const nextWindows = mergedWindows.map((windowInfo) => {
    const windowIndex = normalizeWindowIndex(windowInfo.index)
    if (windowIndex === null) return windowInfo

    let paneCount = paneCountByWindow.get(windowIndex) ?? 0
    if (optimisticCreatedWindowIndexes.has(windowIndex)) {
      paneCount = Math.max(1, paneCount, windowInfo.panes)
    } else if (paneCount === 0) {
      paneCount = windowInfo.panes
    }

    const floor = pendingWindowPaneFloorMap?.get(windowIndex)
    if (typeof floor === 'number' && Number.isFinite(floor) && floor >= 0) {
      const normalizedFloor = Math.trunc(floor)
      if (paneCount >= normalizedFloor) {
        confirmedWindowPaneFloors.push(windowIndex)
      } else {
        paneCount = normalizedFloor
      }
    }

    if (paneCount === windowInfo.panes) {
      return windowInfo
    }
    return { ...windowInfo, panes: paneCount }
  })

  confirmedWindowCreates.sort((left, right) => left - right)
  confirmedWindowCloses.sort((left, right) => left - right)
  confirmedPaneCloses.sort((left, right) => left.localeCompare(right))
  confirmedWindowPaneFloors.sort((left, right) => left - right)

  return {
    windows: nextWindows,
    panes: mergedPanes,
    confirmedWindowCreates,
    confirmedWindowCloses,
    confirmedPaneCloses,
    confirmedWindowPaneFloors,
  }
}
