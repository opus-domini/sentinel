import { Plus, X } from 'lucide-react'
import type { WindowInfo } from '@/types'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

type WindowStripProps = {
  hasActiveSession: boolean
  inspectorLoading: boolean
  inspectorError: string
  windows: Array<WindowInfo>
  activeWindowIndex: number | null
  onSelectWindow: (windowIndex: number) => void
  onCloseWindow: (windowIndex: number) => void
  onRenameWindow: (windowInfo: WindowInfo) => void
  onCreateWindow: () => void
}

export default function WindowStrip({
  hasActiveSession,
  inspectorLoading,
  inspectorError,
  windows,
  activeWindowIndex,
  onSelectWindow,
  onCloseWindow,
  onRenameWindow,
  onCreateWindow,
}: WindowStripProps) {
  const isMobile = useIsMobileLayout()
  const sortedWindows = [...windows].sort(
    (left, right) => left.index - right.index,
  )
  const stripClass = 'flex min-h-[24px] items-center gap-1.5 overflow-x-auto'

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

  return (
    <div className={stripClass}>
      <TooltipHelper content="Create window">
        <Button
          variant="outline"
          size="icon-sm"
          onClick={onCreateWindow}
          aria-label="Create window"
        >
          <Plus className="h-4 w-4" />
        </Button>
      </TooltipHelper>

      {sortedWindows.length === 0 && (
        <span className="truncate">No windows found for this session.</span>
      )}
      {sortedWindows.map((windowInfo) => {
        const isActive = activeWindowIndex === windowInfo.index
        const unreadPanes = windowInfo.unreadPanes ?? 0
        const hasUnread = windowInfo.hasUnread ?? unreadPanes > 0
        return (
          <ContextMenu key={`${windowInfo.session}:${windowInfo.index}`}>
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
                  onClick={() => onSelectWindow(windowInfo.index)}
                  aria-label={
                    isMobile ? `Select window ${windowInfo.name}` : undefined
                  }
                >
                  {isMobile ? windowInfo.index : windowInfo.name}
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
            </ContextMenuTrigger>
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
      })}
    </div>
  )
}
