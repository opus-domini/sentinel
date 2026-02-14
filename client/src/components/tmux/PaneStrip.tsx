import { ArrowLeftRight, ArrowUpDown, X } from 'lucide-react'
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
  const sortedPanes = [...panes].sort((left, right) => {
    if (left.windowIndex !== right.windowIndex) {
      return left.windowIndex - right.windowIndex
    }
    return left.paneIndex - right.paneIndex
  })
  const visiblePanes =
    activeWindowIndex === null
      ? []
      : sortedPanes.filter(
          (paneInfo) => paneInfo.windowIndex === activeWindowIndex,
        )
  const stripClass = 'flex min-h-[24px] items-center gap-1.5 overflow-x-auto'

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
    <div className={stripClass}>
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
                      : 'border-border text-secondary-foreground',
                )}
              >
                <button
                  className="inline-flex cursor-pointer items-center gap-1 px-1.5 py-0.5 whitespace-nowrap hover:text-foreground"
                  type="button"
                  onClick={() => onSelectPane(paneInfo.paneId)}
                >
                  {isMobile ? paneInfo.paneIndex : paneLabel}
                </button>
                {!isMobile && (
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
              <ContextMenuItem onSelect={() => onRenamePane(paneInfo)}>
                Rename pane
              </ContextMenuItem>
              <ContextMenuItem
                className="text-destructive-foreground focus:text-destructive-foreground"
                onSelect={() => onClosePane(paneInfo.paneId)}
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
