import type { WindowInfo } from '@/types'

export function addPendingWindowCreate(
  pendingCreates: Map<string, Set<number>>,
  session: string,
  windowIndex: number,
): void {
  const name = session.trim()
  if (name === '') return
  if (!Number.isFinite(windowIndex) || windowIndex < 0) return
  const index = Math.trunc(windowIndex)
  const existing = pendingCreates.get(name)
  if (existing) {
    existing.add(index)
    return
  }
  pendingCreates.set(name, new Set([index]))
}

export function removePendingWindowCreate(
  pendingCreates: Map<string, Set<number>>,
  session: string,
  windowIndex: number,
): void {
  const name = session.trim()
  if (name === '') return
  if (!Number.isFinite(windowIndex) || windowIndex < 0) return
  const index = Math.trunc(windowIndex)
  const existing = pendingCreates.get(name)
  if (!existing) return
  existing.delete(index)
  if (existing.size === 0) {
    pendingCreates.delete(name)
  }
}

export function clearPendingWindowCreatesForSession(
  pendingCreates: Map<string, Set<number>>,
  session: string,
): void {
  const name = session.trim()
  if (name === '') return
  pendingCreates.delete(name)
}

export function mergePendingWindowCreates(
  session: string,
  windows: Array<WindowInfo>,
  pendingCreates: ReadonlyMap<string, ReadonlySet<number>>,
): {
  windows: Array<WindowInfo>
  confirmedPendingIndexes: Array<number>
} {
  const name = session.trim()
  if (name === '') {
    return { windows, confirmedPendingIndexes: [] }
  }
  const pendingIndexes = pendingCreates.get(name)
  if (!pendingIndexes || pendingIndexes.size === 0) {
    return { windows, confirmedPendingIndexes: [] }
  }

  const existingIndexes = new Set(
    windows.map((windowInfo) => Math.trunc(windowInfo.index)),
  )
  const confirmedPendingIndexes: Array<number> = []
  const mergedWindows = [...windows]

  for (const rawIndex of pendingIndexes) {
    if (!Number.isFinite(rawIndex) || rawIndex < 0) continue
    const index = Math.trunc(rawIndex)
    if (existingIndexes.has(index)) {
      confirmedPendingIndexes.push(index)
      continue
    }
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

  if (mergedWindows.length !== windows.length) {
    mergedWindows.sort((left, right) => left.index - right.index)
  }

  return { windows: mergedWindows, confirmedPendingIndexes }
}
