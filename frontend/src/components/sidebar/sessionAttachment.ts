import type { Session } from '../../types'

export function isSessionAttachedWithLocalTab(
  session: Pick<Session, 'attached'>,
  hasLocalTab: boolean,
): boolean {
  return session.attached > 0 || hasLocalTab
}

export function isSessionAttached(
  session: Pick<Session, 'name' | 'attached'>,
  openTabsSet: ReadonlySet<string>,
): boolean {
  return isSessionAttachedWithLocalTab(session, openTabsSet.has(session.name))
}

export function effectiveAttachedClients(
  attached: number,
  hasLocalTab: boolean,
): number {
  return Math.max(attached, hasLocalTab ? 1 : 0)
}
