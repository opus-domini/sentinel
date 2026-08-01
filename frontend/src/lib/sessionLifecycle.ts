import type { Session } from '@/types'

export function mapSessionLifecycles(sessions: Array<Session>) {
  const lifecycles = new Map<string, NonNullable<Session['lifecycle']>>()
  for (const session of sessions) {
    if (session.lifecycle) {
      lifecycles.set(session.name, session.lifecycle)
    }
  }
  return lifecycles
}
