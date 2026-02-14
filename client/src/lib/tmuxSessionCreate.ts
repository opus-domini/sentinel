import type { Session } from '@/types'

export function buildOptimisticSession(
  name: string,
  at: string,
): Session {
  return {
    name,
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
  }
}

export function upsertOptimisticAttachedSession(
  sessions: Array<Session>,
  sessionName: string,
  at: string,
): Array<Session> {
  const index = sessions.findIndex((item) => item.name === sessionName)
  if (index === -1) {
    return [...sessions, buildOptimisticSession(sessionName, at)]
  }

  const existing = sessions[index]
  const attached = Math.max(1, existing.attached)
  if (attached === existing.attached && existing.activityAt === at) {
    return sessions
  }

  const next = [...sessions]
  next[index] = {
    ...existing,
    attached,
    activityAt: at,
  }
  return next
}

export function mergePendingCreateSessions(
  sessions: Array<Session>,
  pendingCreates: ReadonlyMap<string, string>,
): {
  sessions: Array<Session>
  sessionNamesForSync: Array<string>
  confirmedPendingNames: Array<string>
} {
  const backendNames = new Set(sessions.map((item) => item.name))
  const confirmedPendingNames: Array<string> = []
  let mergedSessions = sessions

  for (const [name, at] of pendingCreates) {
    if (backendNames.has(name)) {
      confirmedPendingNames.push(name)
      continue
    }
    mergedSessions = upsertOptimisticAttachedSession(mergedSessions, name, at)
  }

  const sessionNamesForSync = Array.from(
    new Set([...sessions.map((item) => item.name), ...pendingCreates.keys()]),
  )

  return {
    sessions: mergedSessions,
    sessionNamesForSync,
    confirmedPendingNames,
  }
}
