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

export type ClassifyPatchesResult = {
  hasInputPatches: boolean
  hasUnknownSession: boolean
  applicableNames: Array<string>
}

export function classifySessionPatches(
  rawPatches: Array<SessionActivityPatch> | undefined,
  knownSessionNames: ReadonlySet<string>,
  trackedSessionNames: ReadonlySet<string>,
): ClassifyPatchesResult {
  if (!Array.isArray(rawPatches) || rawPatches.length === 0) {
    return {
      hasInputPatches: false,
      hasUnknownSession: false,
      applicableNames: [],
    }
  }

  let hasInputPatches = false
  let hasUnknownSession = false
  const applicableNames: Array<string> = []
  for (const patch of rawPatches) {
    const name = patch.name?.trim() ?? ''
    if (name === '') continue
    hasInputPatches = true
    if (!knownSessionNames.has(name)) {
      hasUnknownSession = true
      continue
    }
    if (!trackedSessionNames.has(name)) {
      continue
    }
    applicableNames.push(name)
  }
  return { hasInputPatches, hasUnknownSession, applicableNames }
}

export type InspectorProjectionRefreshMode = 'none' | 'windows' | 'full'

export function shouldRefreshSessionsFromEvent(
  actionRaw: string | undefined,
  patchResult: SessionPatchApplyResult,
): { refresh: boolean; minGapMs?: number } {
  const action = (actionRaw ?? '').trim().toLowerCase()
  const { hasInputPatches, applied, hasUnknownSession } = patchResult

  if (action === 'activity' || action === 'seen') {
    if (applied && !hasUnknownSession) {
      return { refresh: false }
    }
    if (hasInputPatches && !hasUnknownSession) {
      // Server already sent patch data; local policy may intentionally
      // skip applying patches for untracked idle sessions.
      return { refresh: false }
    }
    return { refresh: true, minGapMs: 20_000 }
  }

  if (applied && !hasUnknownSession) {
    return { refresh: false }
  }
  return { refresh: true }
}

export function inspectorRefreshModeFromSessionProjection(
  prev: SessionProjectionSnapshot | null,
  next: SessionProjectionSnapshot,
): InspectorProjectionRefreshMode {
  if (prev === null || prev.name !== next.name) {
    return 'none'
  }
  const structureChanged =
    prev.windows !== next.windows || prev.panes !== next.panes
  if (structureChanged) {
    return 'full'
  }
  const unreadCountChanged =
    prev.unreadWindows !== next.unreadWindows ||
    prev.unreadPanes !== next.unreadPanes
  if (unreadCountChanged) {
    return 'windows'
  }
  return 'none'
}

export function shouldRefreshInspectorFromSessionProjection(
  prev: SessionProjectionSnapshot | null,
  next: SessionProjectionSnapshot,
): boolean {
  const mode = inspectorRefreshModeFromSessionProjection(prev, next)
  return mode !== 'none'
}
