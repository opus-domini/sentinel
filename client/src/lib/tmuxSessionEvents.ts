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
