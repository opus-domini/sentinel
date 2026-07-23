import { useCallback } from 'react'
import type { KeyboardEvent } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import {
  SortableContext,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { X } from 'lucide-react'
import type { ClientRect, DragEndEvent, Modifier } from '@dnd-kit/core'
import type { Transform } from '@dnd-kit/utilities'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { useViewport } from '@/contexts/ViewportContext'
import { cn } from '@/lib/utils'
import { hapticFeedback } from '@/lib/device'
import { getTmuxIcon } from '@/lib/tmuxIcons'

type SessionTabsProps = {
  openTabs: Array<string>
  activeSession: string
  activitySessions?: ReadonlySet<string>
  sessionIcons?: ReadonlyMap<string, string>
  onSelect: (session: string) => void
  onClose: (session: string) => void
  onRename?: (session: string) => void
  onKill?: (session: string) => void
  onReorder?: (from: number, to: number) => void
  emptyLabel?: string
}

function canAutoScrollSessionTabs(element: Element): boolean {
  return element instanceof HTMLElement && element.dataset.sentinelSessionTabsScroll === 'true'
}

function clamp(value: number, min: number, max: number): number {
  if (max < min) {
    return min
  }
  return Math.min(Math.max(value, min), max)
}

export function clampSessionTabTransform(
  transform: Transform,
  draggingNodeRect: ClientRect | null,
  scrollableAncestors: Array<Element>,
): Transform {
  const tabsElement = scrollableAncestors.find(canAutoScrollSessionTabs)
  const tabsRect = tabsElement instanceof HTMLElement ? tabsElement.getBoundingClientRect() : null

  if (draggingNodeRect === null || tabsRect === null) {
    return {
      ...transform,
      y: 0,
    }
  }

  const minX = tabsRect.left - draggingNodeRect.left
  const maxX = tabsRect.right - draggingNodeRect.right

  return {
    ...transform,
    x: clamp(transform.x, minX, maxX),
    y: 0,
  }
}

const restrictToSessionTabsBounds: Modifier = ({
  draggingNodeRect,
  scrollableAncestors,
  transform,
}) => clampSessionTabTransform(transform, draggingNodeRect, scrollableAncestors)

function SortableTab({
  tabName,
  iconKey,
  isActive,
  hasActivity,
  showIcon,
  dragEnabled,
  touchOptimized,
  onSelect,
  onClose,
  onRename,
  onKill,
}: {
  tabName: string
  iconKey: string
  isActive: boolean
  hasActivity: boolean
  showIcon: boolean
  dragEnabled: boolean
  touchOptimized: boolean
  onSelect: () => void
  onClose: () => void
  onRename?: () => void
  onKill?: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: tabName,
  })

  const style = {
    transform: dragEnabled ? CSS.Transform.toString(transform) : undefined,
    transition: dragEnabled ? transition : undefined,
    opacity: dragEnabled && isDragging ? 0.5 : undefined,
    zIndex: dragEnabled && isDragging ? 10 : undefined,
    touchAction: dragEnabled ? undefined : ('pan-x' as const),
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onSelect()
    }
  }

  const SessionIcon = getTmuxIcon(iconKey)
  const iconClassName = cn(
    'size-3.5 shrink-0',
    isActive
      ? 'text-primary'
      : hasActivity
        ? 'text-activity-foreground'
        : 'text-secondary-foreground',
  )

  const tabContent = (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'inline-flex h-full min-w-[110px] max-w-[220px] cursor-pointer select-none items-center gap-1.5 border-r border-border-subtle px-2 text-[12px]/none',
        isActive
          ? 'bg-surface-active text-foreground'
          : 'bg-surface-elevated text-secondary-foreground hover:bg-surface-active',
      )}
      {...(dragEnabled ? attributes : {})}
      {...(dragEnabled ? listeners : {})}
      onMouseDown={(event) => {
        event.preventDefault()
      }}
      onClick={onSelect}
      onKeyDown={handleKeyDown}
      role="tab"
      aria-selected={isActive}
      aria-label={hasActivity ? `${tabName}, unread activity` : tabName}
      tabIndex={0}
    >
      {showIcon && <SessionIcon className={iconClassName} />}
      <span className="min-w-0 truncate pt-[5px] pr-2 leading-none">{tabName}</span>
      {!touchOptimized && (
        <Button
          variant="ghost"
          size="icon-xs"
          className="ml-auto h-5 w-5 min-w-0 p-1 text-muted-foreground hover:text-foreground"
          onClick={(event) => {
            event.stopPropagation()
            onClose()
          }}
          aria-label={`Close ${tabName} tab`}
        >
          <X className="h-2.5 w-2.5" />
        </Button>
      )}
    </div>
  )

  if (!onRename && !onKill && !touchOptimized) {
    return tabContent
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{tabContent}</ContextMenuTrigger>
      <ContextMenuContent className="w-44">
        {onRename && (
          <ContextMenuItem
            onSelect={(event) => {
              event.preventDefault()
              onRename()
            }}
          >
            Rename session
          </ContextMenuItem>
        )}
        {onRename && onKill && <ContextMenuSeparator />}
        {onKill && (
          <ContextMenuItem
            className="text-destructive-foreground focus:text-destructive-foreground"
            onSelect={(event) => {
              event.preventDefault()
              onKill()
            }}
          >
            Kill session
          </ContextMenuItem>
        )}
        {touchOptimized && (onRename || onKill) && <ContextMenuSeparator />}
        {touchOptimized && (
          <ContextMenuItem
            className="text-destructive-foreground focus:text-destructive-foreground"
            onSelect={(event) => {
              event.preventDefault()
              onClose()
            }}
          >
            Close tab
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  )
}

export default function SessionTabs({
  openTabs,
  activeSession,
  activitySessions,
  sessionIcons,
  onSelect,
  onClose,
  onRename,
  onKill,
  onReorder,
  emptyLabel = 'No open sessions',
}: SessionTabsProps) {
  const { compactLayout, touchOptimized } = useViewport()
  const dragEnabled = !touchOptimized
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event
      if (!over || active.id === over.id || !onReorder) return

      const from = openTabs.indexOf(active.id as string)
      const to = openTabs.indexOf(over.id as string)
      if (from === -1 || to === -1) return

      hapticFeedback()
      onReorder(from, to)
    },
    [onReorder, openTabs],
  )

  return (
    <div
      role="tablist"
      aria-label="Session tabs"
      className="no-scrollbar flex items-stretch overflow-x-auto overflow-y-hidden border-b border-border bg-surface-sunken"
      data-sentinel-session-tabs-scroll="true"
      style={{
        overscrollBehaviorX: 'contain',
        overscrollBehaviorY: 'none',
      }}
    >
      {openTabs.length === 0 && (
        <div className="inline-flex h-full min-w-[120px] items-center border-r border-border-subtle bg-surface-elevated px-2 text-[12px] text-secondary-foreground">
          {emptyLabel}
        </div>
      )}

      {openTabs.length > 0 && (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          modifiers={[restrictToSessionTabsBounds]}
          autoScroll={{
            enabled: true,
            canScroll: canAutoScrollSessionTabs,
            layoutShiftCompensation: { x: true, y: false },
          }}
          onDragEnd={handleDragEnd}
        >
          <SortableContext items={openTabs} strategy={horizontalListSortingStrategy}>
            {openTabs.map((tabName) => (
              <SortableTab
                key={tabName}
                tabName={tabName}
                iconKey={sessionIcons?.get(tabName) ?? ''}
                isActive={tabName === activeSession}
                hasActivity={tabName !== activeSession && (activitySessions?.has(tabName) ?? false)}
                showIcon={!compactLayout}
                dragEnabled={dragEnabled}
                touchOptimized={touchOptimized}
                onSelect={() => onSelect(tabName)}
                onClose={() => onClose(tabName)}
                onRename={onRename ? () => onRename(tabName) : undefined}
                onKill={onKill ? () => onKill(tabName) : undefined}
              />
            ))}
          </SortableContext>
        </DndContext>
      )}
    </div>
  )
}
