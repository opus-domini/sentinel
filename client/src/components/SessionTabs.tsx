import { useCallback } from 'react'
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
import type { DragEndEvent } from '@dnd-kit/core'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { cn } from '@/lib/utils'
import { hapticFeedback } from '@/lib/device'

type SessionTabsProps = {
  openTabs: Array<string>
  activeSession: string
  onSelect: (session: string) => void
  onClose: (session: string) => void
  onRename?: (session: string) => void
  onKill?: (session: string) => void
  onReorder?: (from: number, to: number) => void
  emptyLabel?: string
}

function SortableTab({
  tabName,
  isActive,
  dragEnabled,
  onSelect,
  onClose,
  onRename,
  onKill,
}: {
  tabName: string
  isActive: boolean
  dragEnabled: boolean
  onSelect: () => void
  onClose: () => void
  onRename?: () => void
  onKill?: () => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: tabName,
  })

  const style = {
    transform: dragEnabled ? CSS.Transform.toString(transform) : undefined,
    transition: dragEnabled ? transition : undefined,
    opacity: dragEnabled && isDragging ? 0.5 : undefined,
    zIndex: dragEnabled && isDragging ? 10 : undefined,
    touchAction: dragEnabled ? undefined : ('pan-x' as const),
  }

  const tabContent = (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'inline-flex h-[30px] min-w-[110px] max-w-[220px] cursor-pointer items-center border-r border-border-subtle px-2 text-[12px]',
        isActive
          ? 'bg-surface-active text-foreground'
          : 'bg-surface-elevated text-secondary-foreground hover:bg-surface-active',
      )}
      onClick={onSelect}
      {...(dragEnabled ? attributes : {})}
      {...(dragEnabled ? listeners : {})}
      role="tab"
      aria-selected={isActive}
    >
      <span className="min-w-0 truncate pr-2">{tabName}</span>
      <Button
        variant="ghost"
        size="icon-xs"
        className="ml-auto h-5 w-5 min-w-0 p-1 text-muted-foreground hover:text-foreground"
        onClick={(event) => {
          event.stopPropagation()
          onClose()
        }}
        aria-label="Close tab"
      >
        <X className="h-2.5 w-2.5" />
      </Button>
    </div>
  )

  if (!onRename && !onKill) {
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
      </ContextMenuContent>
    </ContextMenu>
  )
}

export default function SessionTabs({
  openTabs,
  activeSession,
  onSelect,
  onClose,
  onRename,
  onKill,
  onReorder,
  emptyLabel = 'No open sessions',
}: SessionTabsProps) {
  const isMobile = useIsMobileLayout()
  const dragEnabled = !isMobile
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
    >
      {openTabs.length === 0 && (
        <div className="inline-flex h-[30px] min-w-[120px] items-center border-r border-border-subtle bg-surface-elevated px-2 text-[12px] text-secondary-foreground">
          {emptyLabel}
        </div>
      )}

      {openTabs.length > 0 && (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext
            items={openTabs}
            strategy={horizontalListSortingStrategy}
          >
            {openTabs.map((tabName) => (
              <SortableTab
                key={tabName}
                tabName={tabName}
                isActive={tabName === activeSession}
                dragEnabled={dragEnabled}
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
