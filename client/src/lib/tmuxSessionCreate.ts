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
  pendingKills?: ReadonlySet<string>,
  pendingRenames?: ReadonlyMap<string, string>,
): {
  sessions: Array<Session>
  sessionNamesForSync: Array<string>
  confirmedPendingNames: Array<string>
  confirmedKilledNames: Array<string>
  confirmedRenamedNames: Array<string>
} {
  const backendNames = new Set(sessions.map((item) => item.name.trim()))
  const confirmedPendingNames: Array<string> = []
  const confirmedKilledNames: Array<string> = []
  const confirmedRenamedNames: Array<string> = []
  const pendingKillNames = pendingKills ?? new Set<string>()
  const renameAliases = new Map<string, string>()
  for (const [rawFrom, rawTo] of pendingRenames ?? []) {
    const from = rawFrom.trim()
    const to = rawTo.trim()
    if (from === '' || to === '' || from === to) {
      continue
    }
    const fromExists = backendNames.has(from)
    const toExists = backendNames.has(to)
    if (!fromExists || toExists) {
      confirmedRenamedNames.push(from)
      continue
    }
    renameAliases.set(from, to)
  }

  const backendVisibleNames = new Set(backendNames)
  for (const to of renameAliases.values()) {
    backendVisibleNames.add(to)
  }

  let mergedSessions = sessions
    .map((item) => {
      const nextName = renameAliases.get(item.name.trim())
      if (!nextName) {
        return item
      }
      return { ...item, name: nextName }
    })
    .filter((item) => !pendingKillNames.has(item.name.trim()))

  for (const name of pendingKillNames) {
    if (!backendVisibleNames.has(name)) {
      confirmedKilledNames.push(name)
    }
  }

  for (const [name, at] of pendingCreates) {
    if (pendingKillNames.has(name)) {
      continue
    }
    if (backendVisibleNames.has(name)) {
      confirmedPendingNames.push(name)
      continue
    }
    mergedSessions = upsertOptimisticAttachedSession(mergedSessions, name, at)
  }

  const sessionNamesForSync = Array.from(
    new Set([
      ...mergedSessions
        .map((item) => item.name)
        .filter((name) => !pendingKillNames.has(name)),
      ...Array.from(pendingCreates.keys()).filter(
        (name) => !pendingKillNames.has(name),
      ),
    ]),
  )

  return {
    sessions: mergedSessions,
    sessionNamesForSync,
    confirmedPendingNames,
    confirmedKilledNames,
    confirmedRenamedNames,
  }
}
