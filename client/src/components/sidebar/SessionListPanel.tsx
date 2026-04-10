import { useMemo } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import SessionListItem from './SessionListItem'
import { isSessionAttached } from './sessionAttachment'
import type { SidebarDensity } from '@/contexts/LayoutContext'
import type { Session, SessionPreset } from '../../types'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { hapticFeedback } from '@/lib/device'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

type SessionListPanelProps = {
  sessions: Array<Session>
  tmuxUnavailable: boolean
  openTabs: Array<string>
  activeSession: string
  filter: string
  presets: Array<SessionPreset>
  density?: SidebarDensity
  onFilterChange: (value: string) => void
  onAttach: (session: string) => void
  onRename: (session: string) => void
  onDetach: (session: string) => void
  onKill: (session: string) => void
  onChangeIcon: (session: string, icon: string) => void
  onPinSession: (session: string) => void
  onUnpinSession: (session: string) => void
  onReorder: (activeName: string, overName: string) => void
}

export default function SessionListPanel({
  sessions,
  tmuxUnavailable,
  openTabs,
  activeSession,
  filter,
  presets,
  density = 'compact',
  onFilterChange,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
  onPinSession,
  onUnpinSession,
  onReorder,
}: SessionListPanelProps) {
  const isMobileLayout = useIsMobileLayout()
  const dragEnabled = !isMobileLayout
  const hasFilter = filter.trim() !== ''
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  const openTabsSet = useMemo(() => new Set(openTabs), [openTabs])
  const pinnedNames = useMemo(
    () => new Set(presets.map((preset) => preset.name)),
    [presets],
  )
  const { attachedSessions, idleSessions } = useMemo(() => {
    const attached: Array<Session> = []
    const idle: Array<Session> = []
    for (const session of sessions) {
      if (pinnedNames.has(session.name)) {
        continue
      }
      if (isSessionAttached(session, openTabsSet)) {
        attached.push(session)
      } else {
        idle.push(session)
      }
    }
    return { attachedSessions: attached, idleSessions: idle }
  }, [openTabsSet, pinnedNames, sessions])

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    hapticFeedback()
    onReorder(String(active.id), String(over.id))
  }

  const visibleSessionCount = attachedSessions.length + idleSessions.length
  const allPinned = sessions.length > 0 && pinnedNames.size > 0

  if (visibleSessionCount === 0 && allPinned) {
    return null
  }

  return (
    <section className="rounded-lg border border-border-subtle bg-secondary">
      <ul className="grid min-w-0 grid-cols-1 list-none gap-1.5 overflow-x-hidden p-2">
        {visibleSessionCount === 0 && (
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
          <>
            <li className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
              Attached
            </li>
            <li>
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={attachedSessions.map((session) => session.name)}
                  strategy={verticalListSortingStrategy}
                >
                  <ul className="grid list-none gap-1.5">
                    {attachedSessions.map((session) => (
                      <SessionListItem
                        key={session.name}
                        session={session}
                        isActive={session.name === activeSession}
                        isPinned={false}
                        onAttach={onAttach}
                        onRename={onRename}
                        onDetach={onDetach}
                        onKill={onKill}
                        onChangeIcon={onChangeIcon}
                        onPinSession={onPinSession}
                        onUnpinSession={onUnpinSession}
                        canDetach={openTabsSet.has(session.name)}
                        density={density}
                        dragEnabled={dragEnabled}
                      />
                    ))}
                  </ul>
                </SortableContext>
              </DndContext>
            </li>
          </>
        )}

        {idleSessions.length > 0 && (
          <>
            <li className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
              {attachedSessions.length > 0 ? 'Idle' : 'Sessions'}
            </li>
            <li>
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={idleSessions.map((session) => session.name)}
                  strategy={verticalListSortingStrategy}
                >
                  <ul className="grid list-none gap-1.5">
                    {idleSessions.map((session) => (
                      <SessionListItem
                        key={session.name}
                        session={session}
                        isActive={session.name === activeSession}
                        isPinned={false}
                        onAttach={onAttach}
                        onRename={onRename}
                        onDetach={onDetach}
                        onKill={onKill}
                        onChangeIcon={onChangeIcon}
                        onPinSession={onPinSession}
                        onUnpinSession={onUnpinSession}
                        canDetach={openTabsSet.has(session.name)}
                        density={density}
                        dragEnabled={dragEnabled}
                      />
                    ))}
                  </ul>
                </SortableContext>
              </DndContext>
            </li>
          </>
        )}
      </ul>
    </section>
  )
}
