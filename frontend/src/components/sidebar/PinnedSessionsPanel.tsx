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
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import SessionListItem from './SessionListItem'
import type { SidebarDensity } from '@/contexts/LayoutContext'
import type { Session, SessionPreset } from '@/types'
import { hapticFeedback } from '@/lib/device'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { getTmuxIcon } from '@/lib/tmuxIcons'

type PinnedSessionsPanelProps = {
  sessions: Array<Session>
  presets: Array<SessionPreset>
  filter: string
  openTabs: Array<string>
  activeSession: string
  tmuxUnavailable: boolean
  density?: SidebarDensity
  onAttach: (session: string) => void
  onRename: (session: string) => void
  onDetach: (session: string) => void
  onKill: (session: string) => void
  onChangeIcon: (session: string, icon: string) => void
  onPinSession: (session: string) => void
  onUnpinSession: (session: string) => void
  onLaunchPreset: (name: string) => void
  onReorder: (activeName: string, overName: string) => void
}

function SortablePresetLaunchItem({
  preset,
  tmuxUnavailable,
  dragEnabled = true,
  onLaunchPreset,
}: {
  preset: SessionPreset
  tmuxUnavailable: boolean
  dragEnabled?: boolean
  onLaunchPreset: (name: string) => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: preset.name,
  })
  const PresetIcon = getTmuxIcon(preset.icon)

  return (
    <li
      ref={setNodeRef}
      style={{
        transform: dragEnabled ? CSS.Transform.toString(transform) : undefined,
        transition: dragEnabled ? transition : undefined,
        opacity: dragEnabled && isDragging ? 0.5 : undefined,
        zIndex: dragEnabled && isDragging ? 10 : undefined,
      }}
    >
      <button
        type="button"
        className="flex w-full items-center gap-2 rounded-lg border border-dashed border-border-subtle bg-surface-elevated px-2.5 py-2 text-left transition-colors hover:bg-secondary"
        style={{ touchAction: dragEnabled ? undefined : 'pan-y' }}
        onClick={() => onLaunchPreset(preset.name)}
        disabled={tmuxUnavailable}
        {...(dragEnabled ? attributes : {})}
        {...(dragEnabled ? listeners : {})}
      >
        <PresetIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12px] font-semibold">
            {preset.name}
          </span>
          <span className="block truncate text-[10px] text-muted-foreground">
            {preset.cwd}
            {preset.user && (
              <>
                <span className="mx-1 opacity-40">{'·'}</span>
                <span className="text-primary-text/70">{preset.user}</span>
              </>
            )}
          </span>
        </span>
        <span className="shrink-0 text-[10px] text-muted-foreground">
          Start
        </span>
      </button>
    </li>
  )
}

export default function PinnedSessionsPanel({
  sessions,
  presets,
  filter,
  openTabs,
  activeSession,
  tmuxUnavailable,
  density = 'compact',
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
  onPinSession,
  onUnpinSession,
  onLaunchPreset,
  onReorder,
}: PinnedSessionsPanelProps) {
  const isMobileLayout = useIsMobileLayout()
  const dragEnabled = !isMobileLayout
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )
  const openTabsSet = useMemo(() => new Set(openTabs), [openTabs])
  const normalizedFilter = filter.trim().toLowerCase()
  const sessionsByName = useMemo(
    () => new Map(sessions.map((session) => [session.name, session])),
    [sessions],
  )

  const visiblePresets = useMemo(() => {
    if (normalizedFilter === '') {
      return presets
    }
    return presets.filter((preset) =>
      preset.name.toLowerCase().includes(normalizedFilter),
    )
  }, [normalizedFilter, presets])

  if (visiblePresets.length === 0) {
    return null
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    hapticFeedback()
    onReorder(String(active.id), String(over.id))
  }

  return (
    <section className="rounded-lg border border-border-subtle bg-secondary">
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={visiblePresets.map((preset) => preset.name)}
          strategy={verticalListSortingStrategy}
        >
          <ul className="grid auto-rows-max content-start list-none gap-1.5 p-2">
            <li className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
              Pinned
            </li>
            {visiblePresets.map((preset) => {
              const session = sessionsByName.get(preset.name)
              if (session) {
                return (
                  <SessionListItem
                    key={preset.name}
                    session={session}
                    isActive={session.name === activeSession}
                    isPinned
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
                )
              }

              if (normalizedFilter !== '') {
                return null
              }

              return (
                <SortablePresetLaunchItem
                  key={preset.name}
                  preset={preset}
                  tmuxUnavailable={tmuxUnavailable}
                  dragEnabled={dragEnabled}
                  onLaunchPreset={onLaunchPreset}
                />
              )
            })}
          </ul>
        </SortableContext>
      </DndContext>
    </section>
  )
}
