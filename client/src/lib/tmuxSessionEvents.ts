export type SessionActivityPatch = {
  name?: string
  attached?: number
  windows?: number
  panes?: number
  activityAt?: string
  lastContent?: string
  unreadWindows?: number
  unreadPanes?: number
  rev?: number
}

export type SessionPatchApplyResult = {
  applied: boolean
  hasUnknownSession: boolean
}

export type SessionProjectionSnapshot = {
  name: string
  windows: number
  panes: number
  unreadWindows: number
  unreadPanes: number
}

export function shouldRefreshSessionsFromEvent(
  actionRaw: string | undefined,
  patchResult: SessionPatchApplyResult,
): { refresh: boolean; minGapMs?: number } {
  const action = (actionRaw ?? '').trim().toLowerCase()
  const { applied, hasUnknownSession } = patchResult

  if (action === 'activity' || action === 'seen') {
    if (applied && !hasUnknownSession) {
      return { refresh: false }
    }
    if (!applied) {
      return { refresh: true, minGapMs: 12_000 }
    }
    return { refresh: true, minGapMs: 2_500 }
  }

  if (applied && !hasUnknownSession) {
    return { refresh: false }
  }
  return { refresh: true }
}

export function shouldRefreshInspectorFromSessionProjection(
  prev: SessionProjectionSnapshot | null,
  next: SessionProjectionSnapshot,
): boolean {
  if (prev === null || prev.name !== next.name) {
    return false
  }
  const structureChanged =
    prev.windows !== next.windows || prev.panes !== next.panes
  const unreadCountChanged =
    prev.unreadWindows !== next.unreadWindows ||
    prev.unreadPanes !== next.unreadPanes
  return structureChanged || unreadCountChanged
}
