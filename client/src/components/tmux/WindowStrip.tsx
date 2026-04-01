import { useCallback, useMemo } from 'react'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import {
  SortableContext,
  horizontalListSortingStrategy,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { DragEndEvent } from '@dnd-kit/core'
import type { CSSProperties } from 'react'
import { ChevronDown, Plus, X } from 'lucide-react'
import { getSessionIcon } from '@/components/sidebar/sessionIcons'
import type { TmuxLauncher, WindowInfo } from '@/types'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { TooltipHelper } from '@/components/TooltipHelper'
import { hapticFeedback } from '@/lib/device'
import { cn } from '@/lib/utils'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

function describeLauncherCommand(command: string) {
  const normalized = command.trim()
  if (normalized !== '') {
    return normalized
  }
  return 'plain shell'
}

type WindowStripProps = {
  hasActiveSession: boolean
  inspectorLoading: boolean
  inspectorError: string
  windows: Array<WindowInfo>
  activeWindowIndex: number | null
  launchers: Array<TmuxLauncher>
  recentLauncher: TmuxLauncher | null
  onSelectWindow: (windowIndex: number) => void
  onCloseWindow: (windowIndex: number) => void
  onRenameWindow: (windowInfo: WindowInfo) => void
  onCreateWindow: () => void
  onLaunchLauncher: (launcherID: string) => void
  onOpenLaunchers: () => void
  onReorderWindow?: (activeWindowID: string, overWindowID: string) => void
}

type WindowChipProps = {
  windowInfo: WindowInfo
  isActive: boolean
  isMobile: boolean
  onSelectWindow: (windowIndex: number) => void
  onCloseWindow: (windowIndex: number) => void
  onRenameWindow: (windowInfo: WindowInfo) => void
  containerRef?: (node: HTMLDivElement | null) => void
  containerStyle?: CSSProperties
  dragAttributes?: Record<string, any>
  dragListeners?: Record<string, any>
  isDragging?: boolean
}

function WindowChip({
  windowInfo,
  isActive,
  isMobile,
  onSelectWindow,
  onCloseWindow,
  onRenameWindow,
  containerRef,
  containerStyle,
  dragAttributes,
  dragListeners,
  isDragging = false,
}: WindowChipProps) {
  const unreadPanes = windowInfo.unreadPanes ?? 0
  const hasUnread = windowInfo.hasUnread ?? unreadPanes > 0
  const WindowIcon =
    windowInfo.displayIcon && windowInfo.displayIcon !== ''
      ? getSessionIcon(windowInfo.displayIcon)
      : null

  const content = (
    <div
      ref={containerRef}
      style={containerStyle}
      className={cn(
        'inline-flex max-w-[16rem] shrink-0 items-center overflow-hidden rounded border text-[11px]',
        isActive
          ? 'border-primary/50 text-primary-text'
          : hasUnread
            ? 'border-amber-400/60 text-amber-100'
            : 'border-border text-secondary-foreground',
        isDragging && 'opacity-50',
      )}
    >
      <button
        className={cn(
          'inline-flex min-w-0 items-center gap-1 px-1.5 py-0.5 whitespace-nowrap hover:text-foreground',
          'cursor-pointer',
        )}
        type="button"
        onClick={() => onSelectWindow(windowInfo.index)}
        aria-label={
          isMobile ? `Select window ${windowInfo.displayName}` : undefined
        }
        {...(dragAttributes ?? {})}
        {...(dragListeners ?? {})}
      >
        {isMobile ? (
          windowInfo.index
        ) : (
          <>
            {WindowIcon !== null && <WindowIcon className="h-3.5 w-3.5" />}
            <span className="min-w-0 truncate">{windowInfo.displayName}</span>
          </>
        )}
      </button>
      {!isMobile && (
        <button
          className="grid h-5 w-5 cursor-pointer place-items-center border-l border-border-subtle text-secondary-foreground hover:bg-surface-close-hover hover:text-destructive-foreground"
          type="button"
          onClick={() => onCloseWindow(windowInfo.index)}
          aria-label={`Close window #${windowInfo.index}`}
        >
          <X className="h-3 w-3" />
        </button>
      )}
    </div>
  )

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{content}</ContextMenuTrigger>
      <ContextMenuContent className="w-44">
        <ContextMenuItem onSelect={() => onRenameWindow(windowInfo)}>
          Rename window
        </ContextMenuItem>
        <ContextMenuItem
          className="text-destructive-foreground focus:text-destructive-foreground"
          onSelect={() => onCloseWindow(windowInfo.index)}
        >
          Close window
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}

function SortableWindowChip(props: {
  windowInfo: WindowInfo
  isActive: boolean
  isMobile: boolean
  onSelectWindow: (windowIndex: number) => void
  onCloseWindow: (windowIndex: number) => void
  onRenameWindow: (windowInfo: WindowInfo) => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: props.windowInfo.tmuxWindowId ?? '',
  })

  return (
    <WindowChip
      {...props}
      containerRef={setNodeRef}
      containerStyle={{
        transform: CSS.Transform.toString(transform),
        transition,
        zIndex: isDragging ? 10 : undefined,
      }}
      dragAttributes={attributes}
      dragListeners={listeners}
      isDragging={isDragging}
    />
  )
}

export default function WindowStrip({
  hasActiveSession,
  inspectorLoading,
  inspectorError,
  windows,
  activeWindowIndex,
  launchers,
  recentLauncher,
  onSelectWindow,
  onCloseWindow,
  onRenameWindow,
  onCreateWindow,
  onLaunchLauncher,
  onOpenLaunchers,
  onReorderWindow,
}: WindowStripProps) {
  const isMobile = useIsMobileLayout()
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
  )
  const sortedWindows = useMemo(
    () => [...windows].sort((left, right) => left.index - right.index),
    [windows],
  )
  const secondaryLaunchers = useMemo(
    () =>
      recentLauncher === null
        ? launchers
        : launchers.filter((launcher) => launcher.id !== recentLauncher.id),
    [launchers, recentLauncher],
  )
  const reorderEnabled =
    !isMobile &&
    typeof onReorderWindow === 'function' &&
    sortedWindows.length > 1 &&
    sortedWindows.every(
      (windowInfo) => (windowInfo.tmuxWindowId ?? '').trim() !== '',
    )
  const sortableWindowIDs = useMemo(
    () =>
      reorderEnabled
        ? sortedWindows.map((windowInfo) => windowInfo.tmuxWindowId!.trim())
        : [],
    [reorderEnabled, sortedWindows],
  )
  const stripClass = 'flex min-h-[24px] items-center gap-1.5 overflow-x-auto'

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event
      if (!over || active.id === over.id || !onReorderWindow) {
        return
      }
      hapticFeedback()
      onReorderWindow(String(active.id), String(over.id))
    },
    [onReorderWindow],
  )

  if (!hasActiveSession) {
    return (
      <div className={stripClass}>
        <span className="truncate text-[11px] text-secondary-foreground">
          Select and attach a session.
        </span>
      </div>
    )
  }
  if (inspectorLoading) {
    return (
      <div className={stripClass} aria-busy="true" aria-live="polite">
        <div className="h-6 w-6 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-20 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-24 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <span className="sr-only">Loading windows</span>
      </div>
    )
  }
  if (inspectorError) {
    return (
      <div className={stripClass}>
        <span className="truncate text-[11px] text-destructive-foreground">
          {inspectorError}
        </span>
      </div>
    )
  }

  const windowItems =
    sortedWindows.length === 0 ? (
      <span className="truncate">No windows found for this session.</span>
    ) : reorderEnabled ? (
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={sortableWindowIDs}
          strategy={horizontalListSortingStrategy}
        >
          {sortedWindows.map((windowInfo) => (
            <SortableWindowChip
              key={windowInfo.tmuxWindowId}
              windowInfo={windowInfo}
              isActive={activeWindowIndex === windowInfo.index}
              isMobile={isMobile}
              onSelectWindow={onSelectWindow}
              onCloseWindow={onCloseWindow}
              onRenameWindow={onRenameWindow}
            />
          ))}
        </SortableContext>
      </DndContext>
    ) : (
      sortedWindows.map((windowInfo) => (
        <WindowChip
          key={
            windowInfo.tmuxWindowId ??
            `${windowInfo.session}:${windowInfo.index}`
          }
          windowInfo={windowInfo}
          isActive={activeWindowIndex === windowInfo.index}
          isMobile={isMobile}
          onSelectWindow={onSelectWindow}
          onCloseWindow={onCloseWindow}
          onRenameWindow={onRenameWindow}
        />
      ))
    )

  return (
    <div className={stripClass}>
      <div className="flex shrink-0 items-center">
        <TooltipHelper content="Create blank window">
          <Button
            variant="outline"
            size="icon-sm"
            className="rounded-r-none border-r-0"
            onClick={onCreateWindow}
            aria-label="Create blank window"
          >
            <Plus className="h-4 w-4" />
          </Button>
        </TooltipHelper>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              className="rounded-l-none px-1.5"
              aria-label="Open launcher menu"
            >
              <ChevronDown className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-56">
            <DropdownMenuItem onSelect={onCreateWindow}>
              <Plus className="h-3.5 w-3.5" />
              New blank window
            </DropdownMenuItem>
            {recentLauncher !== null && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel>Last used</DropdownMenuLabel>
                <DropdownMenuItem
                  onSelect={() => onLaunchLauncher(recentLauncher.id)}
                >
                  {(() => {
                    const Icon = getSessionIcon(recentLauncher.icon)
                    return <Icon className="h-3.5 w-3.5" />
                  })()}
                  <span className="flex min-w-0 flex-1 items-center gap-2">
                    <span className="truncate">{recentLauncher.name}</span>
                    <span className="truncate text-[10px] text-muted-foreground">
                      {describeLauncherCommand(recentLauncher.command)}
                    </span>
                  </span>
                </DropdownMenuItem>
              </>
            )}
            {secondaryLaunchers.length > 0 && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel>Launchers</DropdownMenuLabel>
                {secondaryLaunchers.map((launcher) => {
                  const Icon = getSessionIcon(launcher.icon)
                  return (
                    <DropdownMenuItem
                      key={launcher.id}
                      onSelect={() => onLaunchLauncher(launcher.id)}
                    >
                      <Icon className="h-3.5 w-3.5" />
                      <span className="flex min-w-0 flex-1 items-center gap-2">
                        <span className="truncate">{launcher.name}</span>
                        <span className="truncate text-[10px] text-muted-foreground">
                          {describeLauncherCommand(launcher.command)}
                        </span>
                      </span>
                    </DropdownMenuItem>
                  )
                })}
              </>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onOpenLaunchers}>
              Manage launchers...
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {windowItems}
    </div>
  )
}
