import { ArrowLeftRight, ArrowUpDown, X } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type { PaneInfo } from '@/types'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { EmptyState } from '@/components/ui/empty-state'
import { TooltipHelper } from '@/components/TooltipHelper'
import { isPendingSplitPaneID } from '@/lib/tmuxInspectorOptimistic'
import { cn } from '@/lib/utils'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

type PaneStripProps = {
  hasActiveSession: boolean
  inspectorLoading: boolean
  inspectorError: string
  panes: Array<PaneInfo>
  activeWindowIndex: number | null
  activePaneID: string | null
  onSelectPane: (paneID: string) => void
  onClosePane: (paneID: string) => void
  onRenamePane: (paneInfo: PaneInfo) => void
  onSplitPaneVertical: () => void
  onSplitPaneHorizontal: () => void
}

type PaneStripTouchState = {
  id: number
  startX: number
  startY: number
  startScrollLeft: number
  axis: 'unknown' | 'x' | 'y'
}

function parsePendingSplitSlot(paneID: string): number {
  const parts = paneID.trim().split(':')
  const rawSlot = parts.at(-1) ?? ''
  const slot = Number.parseInt(rawSlot, 10)
  if (!Number.isFinite(slot) || slot < 0) {
    return Number.MAX_SAFE_INTEGER
  }
  return slot
}

function parsePaneIDOrder(paneID: string): number | null {
  const trimmed = paneID.trim()
  if (!trimmed.startsWith('%')) return null
  const raw = trimmed.slice(1)
  if (raw === '') return null
  const numeric = Number.parseInt(raw, 10)
  if (!Number.isFinite(numeric) || numeric < 0) return null
  return numeric
}

function findTouchByID(touches: TouchList, id: number): Touch | null {
  for (const touch of Array.from(touches)) {
    if (touch.identifier === id) {
      return touch
    }
  }
  return null
}

export default function PaneStrip({
  hasActiveSession,
  inspectorLoading,
  inspectorError,
  panes,
  activeWindowIndex,
  activePaneID,
  onSelectPane,
  onClosePane,
  onRenamePane,
  onSplitPaneVertical,
  onSplitPaneHorizontal,
}: PaneStripProps) {
  const isMobile = useIsMobileLayout()
  const stripRef = useRef<HTMLDivElement | null>(null)
  const touchStateRef = useRef<PaneStripTouchState | null>(null)
  const sortedPanes = [...panes].sort((left, right) => {
    if (left.windowIndex !== right.windowIndex) {
      return left.windowIndex - right.windowIndex
    }
    const leftPending = isPendingSplitPaneID(left.paneId)
    const rightPending = isPendingSplitPaneID(right.paneId)
    if (leftPending !== rightPending) {
      return leftPending ? 1 : -1
    }
    if (leftPending && rightPending) {
      return (
        parsePendingSplitSlot(left.paneId) - parsePendingSplitSlot(right.paneId)
      )
    }
    const leftIDOrder = parsePaneIDOrder(left.paneId)
    const rightIDOrder = parsePaneIDOrder(right.paneId)
    if (
      leftIDOrder !== null &&
      rightIDOrder !== null &&
      leftIDOrder !== rightIDOrder
    ) {
      return leftIDOrder - rightIDOrder
    }
    return left.paneIndex - right.paneIndex
  })
  const visiblePanes =
    activeWindowIndex === null
      ? []
      : sortedPanes.filter(
          (paneInfo) => paneInfo.windowIndex === activeWindowIndex,
        )
  const stripClass =
    'flex min-h-[24px] items-center gap-1.5 overflow-x-auto overflow-y-hidden'

  useEffect(() => {
    const strip = stripRef.current
    if (!strip) {
      return
    }

    const onTouchStart = (event: TouchEvent) => {
      if (event.touches.length !== 1) {
        touchStateRef.current = null
        return
      }

      const touch = event.touches[0]
      touchStateRef.current = {
        id: touch.identifier,
        startX: touch.clientX,
        startY: touch.clientY,
        startScrollLeft: strip.scrollLeft,
        axis: 'unknown',
      }
    }

    const onTouchMove = (event: TouchEvent) => {
      const state = touchStateRef.current
      if (!state) return

      const touch = findTouchByID(event.touches, state.id)
      if (!touch) return

      const dx = touch.clientX - state.startX
      const dy = touch.clientY - state.startY
      if (state.axis === 'unknown') {
        if (Math.abs(dx) < 3 && Math.abs(dy) < 3) {
          return
        }
        state.axis = Math.abs(dx) >= Math.abs(dy) ? 'x' : 'y'
      }

      // Never let vertical gestures on pane strip bubble into terminal/page scroll.
      if (state.axis === 'y') {
        event.preventDefault()
        event.stopPropagation()
        return
      }

      event.preventDefault()
      strip.scrollLeft = state.startScrollLeft - dx
    }

    const onTouchEnd = (event: TouchEvent) => {
      const state = touchStateRef.current
      if (!state) return
      if (findTouchByID(event.touches, state.id) === null) {
        touchStateRef.current = null
      }
    }

    const onTouchCancel = () => {
      touchStateRef.current = null
    }

    strip.addEventListener('touchstart', onTouchStart, {
      passive: true,
      capture: true,
    })
    strip.addEventListener('touchmove', onTouchMove, {
      passive: false,
      capture: true,
    })
    strip.addEventListener('touchend', onTouchEnd, {
      passive: true,
      capture: true,
    })
    strip.addEventListener('touchcancel', onTouchCancel, {
      passive: true,
      capture: true,
    })

    return () => {
      strip.removeEventListener('touchstart', onTouchStart, true)
      strip.removeEventListener('touchmove', onTouchMove, true)
      strip.removeEventListener('touchend', onTouchEnd, true)
      strip.removeEventListener('touchcancel', onTouchCancel, true)
    }
  }, [])

  if (!hasActiveSession) {
    return (
      <div className={stripClass}>
        <span className="truncate">
          Select and attach a session to inspect panes.
        </span>
      </div>
    )
  }
  if (inspectorLoading) {
    return (
      <div className={stripClass} aria-busy="true" aria-live="polite">
        <div className="h-5 w-5 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-5 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-20 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <div className="h-5 w-24 shrink-0 rounded border border-border-subtle bg-surface-elevated motion-safe:animate-pulse" />
        <span className="sr-only">Loading panes</span>
      </div>
    )
  }
  if (inspectorError) {
    return (
      <div className={stripClass}>
        <span className="truncate text-destructive-foreground">
          {inspectorError}
        </span>
      </div>
    )
  }

  return (
    <div
      ref={stripRef}
      className={stripClass}
      data-sentinel-touch-lock
      style={{
        touchAction: 'none',
        overscrollBehaviorX: 'contain',
        overscrollBehaviorY: 'none',
      }}
    >
      <TooltipHelper content="Split vertical (left/right)">
        <Button
          variant="outline"
          size="icon-xs"
          onClick={onSplitPaneVertical}
          aria-label="Split vertical"
          disabled={visiblePanes.length === 0}
        >
          <ArrowLeftRight className="h-3.5 w-3.5" />
        </Button>
      </TooltipHelper>
      <TooltipHelper content="Split horizontal (top/bottom)">
        <Button
          variant="outline"
          size="icon-xs"
          onClick={onSplitPaneHorizontal}
          aria-label="Split horizontal"
          disabled={visiblePanes.length === 0}
        >
          <ArrowUpDown className="h-3.5 w-3.5" />
        </Button>
      </TooltipHelper>

      {visiblePanes.length === 0 && (
        <EmptyState variant="inline" className="text-[11px]">
          {activeWindowIndex === null
            ? 'Select a window to inspect panes.'
            : `No panes found for window #${activeWindowIndex}.`}
        </EmptyState>
      )}
      {visiblePanes.map((paneInfo) => {
        const isActive = activePaneID === paneInfo.paneId
        const hasUnread = paneInfo.hasUnread ?? false
        const isPending = isPendingSplitPaneID(paneInfo.paneId)
        const canInteract = !isPending
        const paneLabel =
          paneInfo.title.trim() !== '' ? paneInfo.title : paneInfo.paneId
        return (
          <ContextMenu key={paneInfo.paneId}>
            <ContextMenuTrigger asChild>
              <div
                className={cn(
                  'inline-flex shrink-0 items-center overflow-hidden rounded border text-[11px]',
                  isActive
                    ? 'border-primary/50 text-primary-text'
                    : hasUnread
                      ? 'border-amber-400/60 text-amber-100'
                      : isPending
                        ? 'border-border-subtle text-muted-foreground'
                        : 'border-border text-secondary-foreground',
                )}
              >
                <button
                  className={cn(
                    'inline-flex items-center gap-1 px-1.5 py-0.5 whitespace-nowrap',
                    canInteract
                      ? 'cursor-pointer hover:text-foreground'
                      : 'cursor-default opacity-80',
                  )}
                  type="button"
                  disabled={!canInteract}
                  onClick={() => {
                    if (!canInteract) return
                    onSelectPane(paneInfo.paneId)
                  }}
                >
                  {isMobile ? paneInfo.paneIndex : paneLabel}
                </button>
                {!isMobile && canInteract && (
                  <button
                    className="grid h-5 w-5 cursor-pointer place-items-center border-l border-border-subtle text-secondary-foreground hover:bg-surface-close-hover hover:text-destructive-foreground"
                    type="button"
                    onClick={() => onClosePane(paneInfo.paneId)}
                    aria-label={`Close pane ${paneInfo.paneId}`}
                  >
                    <X className="h-3 w-3" />
                  </button>
                )}
              </div>
            </ContextMenuTrigger>
            <ContextMenuContent className="w-44">
              <ContextMenuItem
                disabled={!canInteract}
                onSelect={() => {
                  if (!canInteract) return
                  onRenamePane(paneInfo)
                }}
              >
                Rename pane
              </ContextMenuItem>
              <ContextMenuItem
                disabled={!canInteract}
                className="text-destructive-foreground focus:text-destructive-foreground"
                onSelect={() => {
                  if (!canInteract) return
                  onClosePane(paneInfo.paneId)
                }}
              >
                Close pane
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        )
      })}
    </div>
  )
}
