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

  if (!hasActiveSession) {
    return (
      <span className="truncate text-[11px] text-secondary-foreground">
        Select and attach a session.
      </span>
    )
  }
  if (inspectorLoading) {
    return (
      <span className="truncate text-[11px] text-secondary-foreground">
        Loading windows...
      </span>
    )
  }
  if (inspectorError) {
    return (
      <span className="truncate text-[11px] text-destructive-foreground">
        {inspectorError}
      </span>
    )
  }

  return (
    <div className="flex items-center gap-1.5 overflow-x-auto">
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
        return (
          <ContextMenu key={`${windowInfo.session}:${windowInfo.index}`}>
            <ContextMenuTrigger asChild>
              <div
                className={cn(
                  'inline-flex shrink-0 items-center overflow-hidden rounded border text-[11px]',
                  isActive
                    ? 'border-primary/50 text-primary-text'
                    : 'border-border text-secondary-foreground',
                )}
              >
                <button
                  className="cursor-pointer px-1.5 py-0.5 whitespace-nowrap hover:text-foreground"
                  type="button"
                  onClick={() => onSelectWindow(windowInfo.index)}
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
