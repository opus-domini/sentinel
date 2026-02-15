import type { PaneInfo, WindowInfo } from '@/types'

export type PendingWindowIndexMap = Map<string, Set<number>>
export type PendingPaneIDMap = Map<string, Set<string>>
export type PendingWindowPaneFloorMap = Map<string, Map<number, number>>
const pendingSplitPanePrefix = '__pending_split__'

export function buildPendingSplitPaneID(
  session: string,
  windowIndex: number,
  slot: number,
): string {
  const name = normalizeSession(session)
  const index = normalizeWindowIndex(windowIndex)
  if (name === '' || index === null || !Number.isFinite(slot) || slot < 0) {
    return `${pendingSplitPanePrefix}:invalid`
  }
  return `${pendingSplitPanePrefix}:${name}:${index}:${Math.trunc(slot)}`
}

export function isPendingSplitPaneID(paneID: string): boolean {
  return paneID.trim().startsWith(`${pendingSplitPanePrefix}:`)
}

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
  optimisticVisibleWindowBaseline?: number
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
  const unmatchedPendingCreateIndexes: Array<number> = []
  const pendingCreateIndexes = Array.from(pendingWindowCreateSet ?? [])
    .map((rawIndex) => normalizeWindowIndex(rawIndex))
    .filter((index): index is number => index !== null)
    .sort((left, right) => left - right)

  for (const index of pendingCreateIndexes) {
    if (blockedWindowIndexes.has(index)) continue
    if (backendWindowIndexes.has(index)) {
      confirmedWindowCreates.push(index)
      continue
    }
    unmatchedPendingCreateIndexes.push(index)
  }

  const optimisticVisibleWindowBaseline =
    typeof options.optimisticVisibleWindowBaseline === 'number' &&
    Number.isFinite(options.optimisticVisibleWindowBaseline) &&
    options.optimisticVisibleWindowBaseline >= 0
      ? Math.trunc(options.optimisticVisibleWindowBaseline)
      : null
  let fallbackConfirmedCreates = 0
  if (optimisticVisibleWindowBaseline !== null) {
    const realizedCreateCount = Math.max(
      0,
      mergedWindows.length - optimisticVisibleWindowBaseline,
    )
    fallbackConfirmedCreates = Math.max(
      0,
      realizedCreateCount - confirmedWindowCreates.length,
    )
  }

  for (const index of unmatchedPendingCreateIndexes) {
    if (fallbackConfirmedCreates > 0) {
      confirmedWindowCreates.push(index)
      fallbackConfirmedCreates -= 1
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
  const paneIDs = new Set(
    mergedPanes
      .map((paneInfo) => paneInfo.paneId.trim())
      .filter((id) => id !== ''),
  )
  const paneCountByWindow = new Map<number, number>()
  const realPaneCountByWindow = new Map<number, number>()
  const paneMaxIndexByWindow = new Map<number, number>()
  for (const paneInfo of mergedPanes) {
    const windowIndex = normalizeWindowIndex(paneInfo.windowIndex)
    if (windowIndex === null) continue
    paneCountByWindow.set(
      windowIndex,
      (paneCountByWindow.get(windowIndex) ?? 0) + 1,
    )
    if (!isPendingSplitPaneID(paneInfo.paneId)) {
      realPaneCountByWindow.set(
        windowIndex,
        (realPaneCountByWindow.get(windowIndex) ?? 0) + 1,
      )
    }
    const paneIndex = normalizeWindowIndex(paneInfo.paneIndex) ?? 0
    paneMaxIndexByWindow.set(
      windowIndex,
      Math.max(paneMaxIndexByWindow.get(windowIndex) ?? -1, paneIndex),
    )
  }

  const confirmedWindowPaneFloors: Array<number> = []
  for (const windowInfo of mergedWindows) {
    const windowIndex = normalizeWindowIndex(windowInfo.index)
    if (windowIndex === null) continue
    const floor = pendingWindowPaneFloorMap?.get(windowIndex)
    if (typeof floor !== 'number' || !Number.isFinite(floor) || floor < 0) {
      continue
    }
    const normalizedFloor = Math.trunc(floor)
    const currentRealPaneCount = realPaneCountByWindow.get(windowIndex) ?? 0
    const currentPaneCount = paneCountByWindow.get(windowIndex) ?? 0
    if (currentRealPaneCount >= normalizedFloor) {
      confirmedWindowPaneFloors.push(windowIndex)
      continue
    }

    let nextPaneIndex = (paneMaxIndexByWindow.get(windowIndex) ?? -1) + 1
    for (
      let offset = 0;
      offset < normalizedFloor - currentPaneCount;
      offset += 1
    ) {
      const slot = currentPaneCount + offset
      const pendingPaneID = buildPendingSplitPaneID(name, windowIndex, slot)
      if (paneIDs.has(pendingPaneID)) {
        continue
      }
      paneIDs.add(pendingPaneID)
      mergedPanes.push({
        session: name,
        windowIndex,
        paneIndex: nextPaneIndex,
        paneId: pendingPaneID,
        title: 'new',
        active: false,
        tty: '',
        hasUnread: false,
      })
      nextPaneIndex += 1
    }
    paneCountByWindow.set(windowIndex, normalizedFloor)
    paneMaxIndexByWindow.set(windowIndex, nextPaneIndex - 1)
  }

  if (mergedPanes.length > 1) {
    mergedPanes.sort((left, right) => {
      if (left.windowIndex !== right.windowIndex) {
        return left.windowIndex - right.windowIndex
      }
      return left.paneIndex - right.paneIndex
    })
  }

  const nextWindows = mergedWindows.map((windowInfo) => {
    const windowIndex = normalizeWindowIndex(windowInfo.index)
    if (windowIndex === null) return windowInfo

    let paneCount = paneCountByWindow.get(windowIndex) ?? 0
    if (optimisticCreatedWindowIndexes.has(windowIndex)) {
      paneCount = Math.max(1, paneCount, windowInfo.panes)
    } else if (paneCount === 0) {
      paneCount = windowInfo.panes
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
