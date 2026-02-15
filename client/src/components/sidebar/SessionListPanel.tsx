import { useMemo } from 'react'
import SessionListItem from './SessionListItem'
import { isSessionAttached } from './sessionAttachment'
import type { Session } from '../../types'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'

type SessionListPanelProps = {
  sessions: Array<Session>
  tmuxUnavailable: boolean
  openTabs: Array<string>
  activeSession: string
  filter: string
  onFilterChange: (value: string) => void
  onAttach: (session: string) => void
  onRename: (session: string) => void
  onDetach: (session: string) => void
  onKill: (session: string) => void
  onChangeIcon: (session: string, icon: string) => void
}

export default function SessionListPanel({
  sessions,
  tmuxUnavailable,
  openTabs,
  activeSession,
  filter,
  onFilterChange,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
}: SessionListPanelProps) {
  const sortedSessions = useMemo(() => {
    const next = [...sessions]
    next.sort((left, right) =>
      left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }),
    )
    return next
  }, [sessions])

  const hasFilter = filter.trim() !== ''

  const openTabsSet = useMemo(() => new Set(openTabs), [openTabs])
  const attachedSessions = sortedSessions.filter((session) =>
    isSessionAttached(session, openTabsSet),
  )
  const idleSessions = sortedSessions.filter(
    (session) => !isSessionAttached(session, openTabsSet),
  )

  return (
    <section className="h-full min-h-0 overflow-hidden rounded-lg border border-border-subtle bg-secondary">
      <ul className="grid min-h-0 min-w-0 grid-cols-1 list-none gap-1.5 overflow-x-hidden overflow-y-auto p-2">
        {sessions.length === 0 && (
          <li>
            <EmptyState variant="inline" className="grid gap-1 p-3">
              <span className="text-[12px]">
                {hasFilter
                  ? 'No sessions match filter.'
                  : tmuxUnavailable
                    ? 'tmux is not installed on this host.'
                    : 'No tmux sessions found.'}
              </span>
              {hasFilter && (
                <Button
                  variant="outline"
                  className="mx-auto"
                  type="button"
                  onClick={() => onFilterChange('')}
                >
                  Clear Filter
                </Button>
              )}
            </EmptyState>
          </li>
        )}

        {attachedSessions.length > 0 && (
          <li className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            Attached
          </li>
        )}
        {attachedSessions.map((session) => (
          <SessionListItem
            key={session.name}
            session={session}
            isActive={session.name === activeSession}
            onAttach={onAttach}
            onRename={onRename}
            onDetach={onDetach}
            onKill={onKill}
            onChangeIcon={onChangeIcon}
            canDetach={openTabsSet.has(session.name)}
          />
        ))}

        {idleSessions.length > 0 && (
          <li className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            {attachedSessions.length > 0 ? 'Idle' : 'Sessions'}
          </li>
        )}
        {idleSessions.map((session) => (
          <SessionListItem
            key={session.name}
            session={session}
            isActive={session.name === activeSession}
            onAttach={onAttach}
            onRename={onRename}
            onDetach={onDetach}
            onKill={onKill}
            onChangeIcon={onChangeIcon}
            canDetach={openTabsSet.has(session.name)}
          />
        ))}
      </ul>
    </section>
  )
}
