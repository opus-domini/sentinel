import { useCallback, useMemo } from 'react'
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
import type { DragEndEvent, Modifier, ClientRect } from '@dnd-kit/core'
import type { CSSProperties, WheelEvent as ReactWheelEvent } from 'react'
import type { Transform } from '@dnd-kit/utilities'
import { ChevronDown, Plus, User, X } from 'lucide-react'
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
import { getTmuxIcon } from '@/lib/tmuxIcons'
import { cn } from '@/lib/utils'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

function asText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function describeLauncherCommand(command: unknown) {
  const normalized = asText(command).trim()
  if (normalized !== '') {
    return normalized
  }
  return 'plain shell'
}

function canAutoScrollWindowStrip(element: Element): boolean {
  return (
    element instanceof HTMLElement &&
    element.dataset.sentinelWindowStripScroll === 'true'
  )
}

function clamp(value: number, min: number, max: number): number {
  if (max < min) {
    return min
  }
  return Math.min(Math.max(value, min), max)
}

export function clampWindowStripTransform(
  transform: Transform,
  draggingNodeRect: ClientRect | null,
  scrollableAncestors: Array<Element>,
): Transform {
  const stripElement = scrollableAncestors.find(canAutoScrollWindowStrip)
  const stripRect =
    stripElement instanceof HTMLElement
      ? stripElement.getBoundingClientRect()
      : null

  if (draggingNodeRect === null || stripRect === null) {
    return {
      ...transform,
      y: 0,
    }
  }

  const minX = stripRect.left - draggingNodeRect.left
  const maxX = stripRect.right - draggingNodeRect.right

  return {
    ...transform,
    x: clamp(transform.x, minX, maxX),
    y: 0,
  }
}

const restrictToWindowStripBounds: Modifier = ({
  draggingNodeRect,
  scrollableAncestors,
  transform,
}) =>
  clampWindowStripTransform(transform, draggingNodeRect, scrollableAncestors)

type WindowStripProps = {
  hasActiveSession: boolean
  inspectorLoading: boolean
  inspectorError: string
  windows: Array<WindowInfo>
  activeWindowIndex: number | null
  sessionUser?: string
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
  sessionUser?: string
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
  sessionUser,
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
  const displayName = (() => {
    const normalizedDisplayName = asText(windowInfo.displayName).trim()
    if (normalizedDisplayName !== '') {
      return normalizedDisplayName
    }
    const normalizedName = asText(windowInfo.name).trim()
    if (normalizedName !== '') {
      return normalizedName
    }
    return `#${windowInfo.index}`
  })()
  const WindowIcon =
    asText(windowInfo.displayIcon) !== ''
      ? getTmuxIcon(asText(windowInfo.displayIcon))
      : null

  const content = (
    <div
      ref={containerRef}
      style={containerStyle}
      className={cn(
        'inline-flex max-w-[16rem] shrink-0 items-center overflow-hidden rounded border text-[11px]/none',
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
          'inline-flex h-5 min-w-0 items-center gap-1 px-1.5 whitespace-nowrap leading-none hover:text-foreground',
          'cursor-pointer',
        )}
        type="button"
        onClick={() => onSelectWindow(windowInfo.index)}
        aria-label={isMobile ? `Select window ${displayName}` : undefined}
        {...(dragAttributes ?? {})}
        {...(dragListeners ?? {})}
      >
        {isMobile ? (
          windowInfo.index
        ) : (
          <>
            {windowInfo.user && windowInfo.user !== sessionUser && (
              <TooltipHelper content={`Running as: ${windowInfo.user}`}>
                <User className="h-2.5 w-2.5 shrink-0 text-primary-text/60" />
              </TooltipHelper>
            )}
            {WindowIcon !== null && (
              <WindowIcon className="size-3.5 shrink-0" />
            )}
            <span className="min-w-0 truncate pt-[3px] leading-none">
              {displayName}
            </span>
            {hasUnread && unreadPanes > 1 && (
              <span className="ml-0.5 text-[9px] text-warning-foreground">
                {unreadPanes}
              </span>
            )}
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
  sessionUser?: string
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
  sessionUser,
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
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
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
      (windowInfo) => asText(windowInfo.tmuxWindowId).trim() !== '',
    )
  const sortableWindowIDs = useMemo(
    () =>
      reorderEnabled
        ? sortedWindows.map((windowInfo) =>
            asText(windowInfo.tmuxWindowId).trim(),
          )
        : [],
    [reorderEnabled, sortedWindows],
  )
  const stripClass =
    'no-scrollbar flex min-h-[24px] items-center gap-1.5 overflow-x-auto overflow-y-hidden'
  const hasRenderableWindows = sortedWindows.length > 0

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

  const handleWheel = useCallback((event: ReactWheelEvent<HTMLDivElement>) => {
    const strip = event.currentTarget
    if (strip.scrollWidth <= strip.clientWidth || event.ctrlKey) {
      return
    }

    const horizontalDelta =
      Math.abs(event.deltaX) > Math.abs(event.deltaY)
        ? event.deltaX
        : event.deltaY

    if (horizontalDelta === 0) {
      return
    }

    event.preventDefault()
    strip.scrollLeft += horizontalDelta
  }, [])

  if (!hasActiveSession) {
    return (
      <div className={stripClass}>
        <span className="truncate text-[11px] text-secondary-foreground">
          Select and attach a session.
        </span>
      </div>
    )
  }
  if (inspectorLoading && !hasRenderableWindows) {
    return (
      <div className={stripClass} aria-busy="true" aria-live="polite">
        <div className="h-6 w-6 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-20 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-24 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <span className="sr-only">Loading windows</span>
      </div>
    )
  }
  if (inspectorError && !hasRenderableWindows) {
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
        modifiers={[restrictToWindowStripBounds]}
        autoScroll={{
          enabled: true,
          canScroll: canAutoScrollWindowStrip,
          layoutShiftCompensation: { x: true, y: false },
        }}
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
              sessionUser={sessionUser}
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
          sessionUser={sessionUser}
          onSelectWindow={onSelectWindow}
          onCloseWindow={onCloseWindow}
          onRenameWindow={onRenameWindow}
        />
      ))
    )

  return (
    <div
      className={stripClass}
      data-sentinel-window-strip-scroll="true"
      onWheel={handleWheel}
      style={{
        overscrollBehaviorX: 'contain',
        overscrollBehaviorY: 'none',
      }}
    >
      <div className="flex shrink-0 items-center text-[11px]/none">
        <TooltipHelper content="Create blank window">
          <Button
            variant="outline"
            size="icon-xs"
            className="rounded-r-none border-r-0"
            onClick={onCreateWindow}
            aria-label="Create blank window"
          >
            <Plus className="size-3" />
          </Button>
        </TooltipHelper>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="outline"
              size="icon-xs"
              className="rounded-l-none px-0"
              aria-label="Open launcher menu"
            >
              <ChevronDown className="size-3" />
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
                    const Icon = getTmuxIcon(recentLauncher.icon)
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
                  const Icon = getTmuxIcon(launcher.icon)
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
