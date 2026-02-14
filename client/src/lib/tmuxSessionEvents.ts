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
  hasInputPatches: boolean
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
  const { hasInputPatches, applied, hasUnknownSession } = patchResult

  if (action === 'activity' || action === 'seen') {
    if (hasUnknownSession) {
      return { refresh: true, minGapMs: 2_500 }
    }
    if (applied) {
      return { refresh: false }
    }
    if (hasInputPatches) {
      // Server already sent patch data; local policy may intentionally
      // skip applying patches for untracked idle sessions.
      return { refresh: false }
    }
    return { refresh: true, minGapMs: 12_000 }
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
  const unreadEdgeChanged =
    (prev.unreadWindows > 0) !== (next.unreadWindows > 0) ||
    (prev.unreadPanes > 0) !== (next.unreadPanes > 0)
  return structureChanged || unreadEdgeChanged
}
