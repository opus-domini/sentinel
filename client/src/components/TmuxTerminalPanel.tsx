import { Archive, History, Menu, Minus, Plus, ShieldAlert } from 'lucide-react'
import { useCallback, useEffect, useRef } from 'react'
import ConnectionBadge from './ConnectionBadge'
import SessionTabs from './SessionTabs'
import { TooltipHelper } from './TooltipHelper'
import TerminalControls from './terminal/TerminalControls'
import PaneStrip from './tmux/PaneStrip'
import TerminalHost from './tmux/TerminalHost'
import WindowStrip from './tmux/WindowStrip'
import type { RefCallback } from 'react'
import type { ConnectionState, PaneInfo, WindowInfo } from '../types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

type TmuxTerminalPanelProps = {
  connectionState: ConnectionState
  statusDetail: string
  openTabs: Array<string>
  activeSession: string
  inspectorLoading: boolean
  inspectorError: string
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
  activeWindowIndex: number | null
  activePaneID: string | null
  termCols: number
  termRows: number
  getTerminalHostRef: (session: string) => RefCallback<HTMLDivElement>
  onToggleSidebarOpen: () => void
  onSelectWindow: (windowIndex: number) => void
  onSelectPane: (paneID: string) => void
  onRenameWindow: (windowInfo: WindowInfo) => void
  onRenamePane: (paneInfo: PaneInfo) => void
  onCreateWindow: () => void
  onCloseWindow: (windowIndex: number) => void
  onSplitPaneVertical: () => void
  onSplitPaneHorizontal: () => void
  onClosePane: (paneID: string) => void
  onRenameTab: (session: string) => void
  onKillTab: (session: string) => void
  onSelectTab: (session: string) => void
  onCloseTab: (session: string) => void
  onReorderTabs?: (from: number, to: number) => void
  onSendKey?: (data: string) => void
  onFlushComposition?: () => void
  onFocusTerminal?: () => void
  onZoomIn?: () => void
  onZoomOut?: () => void
  onOpenGuardrails?: () => void
  onOpenSnapshots?: () => void
  onOpenTimeline?: () => void
  onOpenCreateSession?: () => void
}

export default function TmuxTerminalPanel({
  connectionState,
  statusDetail: _statusDetail,
  openTabs,
  activeSession,
  inspectorLoading,
  inspectorError,
  windows,
  panes,
  activeWindowIndex,
  activePaneID,
  termCols,
  termRows,
  getTerminalHostRef,
  onToggleSidebarOpen,
  onSelectWindow,
  onSelectPane,
  onRenameWindow,
  onRenamePane,
  onCreateWindow,
  onCloseWindow,
  onSplitPaneVertical,
  onSplitPaneHorizontal,
  onClosePane,
  onRenameTab,
  onKillTab,
  onSelectTab,
  onCloseTab,
  onReorderTabs,
  onSendKey,
  onFlushComposition,
  onFocusTerminal,
  onZoomIn,
  onZoomOut,
  onOpenGuardrails,
  onOpenSnapshots,
  onOpenTimeline,
  onOpenCreateSession,
}: TmuxTerminalPanelProps) {
  const isMobileLayout = useIsMobileLayout()
  const lockedTouchIDsRef = useRef<Set<number>>(new Set())
  const hasActiveSession = activeSession !== ''
  const showControls =
    isMobileLayout && hasActiveSession && !!onSendKey && !!onFocusTerminal

  const isKeyboardVisible = useCallback(() => {
    if (document.documentElement.classList.contains('keyboard-visible'))
      return true
    const el = document.activeElement as HTMLElement | null
    if (!el) return false
    const tag = el.tagName.toLowerCase()
    return tag === 'textarea' || tag === 'input' || el.isContentEditable
  }, [])

  useEffect(() => {
    if (!isMobileLayout) {
      return
    }

    const lockedTouchIDs = lockedTouchIDsRef.current
    const isLockedTarget = (target: EventTarget | null): boolean =>
      target instanceof Element &&
      target.closest('[data-sentinel-touch-lock]') !== null

    const onTouchStart = (event: TouchEvent) => {
      const lockGesture = isLockedTarget(event.target)
      for (const touch of Array.from(event.changedTouches)) {
        if (lockGesture) {
          lockedTouchIDs.add(touch.identifier)
          continue
        }
        lockedTouchIDs.delete(touch.identifier)
      }
    }

    const onTouchMove = (event: TouchEvent) => {
      for (const touch of Array.from(event.changedTouches)) {
        if (!lockedTouchIDs.has(touch.identifier)) {
          continue
        }
        event.preventDefault()
        return
      }
    }

    const clearTouches = (event: TouchEvent) => {
      for (const touch of Array.from(event.changedTouches)) {
        lockedTouchIDs.delete(touch.identifier)
      }
    }

    document.addEventListener('touchstart', onTouchStart, {
      passive: true,
      capture: true,
    })
    document.addEventListener('touchmove', onTouchMove, {
      passive: false,
      capture: true,
    })
    document.addEventListener('touchend', clearTouches, {
      passive: true,
      capture: true,
    })
    document.addEventListener('touchcancel', clearTouches, {
      passive: true,
      capture: true,
    })

    return () => {
      lockedTouchIDs.clear()
      document.removeEventListener('touchstart', onTouchStart, true)
      document.removeEventListener('touchmove', onTouchMove, true)
      document.removeEventListener('touchend', clearTouches, true)
      document.removeEventListener('touchcancel', clearTouches, true)
    }
  }, [isMobileLayout])

  return (
    <main
      className={cn(
        'grid min-h-0 min-w-0 grid-cols-1 overflow-hidden bg-[radial-gradient(circle_at_20%_-10%,rgba(30,64,175,.18),transparent_34%),var(--background)]',
        showControls
          ? 'grid-rows-[40px_30px_1fr_auto_28px]'
          : 'grid-rows-[40px_30px_1fr_28px]',
      )}
    >
      <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden"
            onClick={onToggleSidebarOpen}
            aria-label="Open menu"
          >
            <Menu className="h-5 w-5" />
          </Button>
          <span className="truncate">Sentinel</span>
          <span className="text-muted-foreground">/</span>
          <span className="truncate text-muted-foreground">tmux</span>
        </div>
        <div className="flex items-center gap-1">
          <TooltipHelper content="Guardrails">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer"
              onClick={onOpenGuardrails}
              disabled={!onOpenGuardrails}
              aria-label="Guardrails"
            >
              <ShieldAlert className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Recovery snapshots">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer"
              onClick={onOpenSnapshots}
              disabled={!onOpenSnapshots}
              aria-label="Recovery snapshots"
            >
              <Archive className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Timeline">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer"
              onClick={onOpenTimeline}
              disabled={!onOpenTimeline}
              aria-label="Timeline"
            >
              <History className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <ConnectionBadge state={connectionState} />
        </div>
      </header>

      <SessionTabs
        openTabs={openTabs}
        activeSession={activeSession}
        onSelect={onSelectTab}
        onClose={onCloseTab}
        onRename={onRenameTab}
        onKill={onKillTab}
        onReorder={onReorderTabs}
      />

      <section
        className={cn(
          'min-h-0 min-w-0 border-x border-border-subtle',
          hasActiveSession
            ? 'grid grid-cols-1 grid-rows-[36px_1fr]'
            : 'flex items-center justify-center',
        )}
      >
        {hasActiveSession ? (
          <>
            <div className="min-w-0 overflow-hidden border-b border-border-subtle bg-surface-overlay px-2.5 py-1">
              <WindowStrip
                hasActiveSession={hasActiveSession}
                inspectorLoading={inspectorLoading}
                inspectorError={inspectorError}
                windows={windows}
                activeWindowIndex={activeWindowIndex}
                onSelectWindow={(idx) => {
                  onSelectWindow(idx)
                  onFocusTerminal?.()
                }}
                onCloseWindow={onCloseWindow}
                onRenameWindow={onRenameWindow}
                onCreateWindow={onCreateWindow}
              />
            </div>

            <div className="relative min-h-0 overflow-hidden">
              <TerminalHost
                openTabs={openTabs}
                activeSession={activeSession}
                getTerminalHostRef={getTerminalHostRef}
              />
            </div>
          </>
        ) : (
          <div className="text-center">
            <p className="text-[13px] text-muted-foreground">
              Attach to a session from the sidebar
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-3 h-7 cursor-pointer text-[11px]"
              onClick={onOpenCreateSession}
            >
              Create new session
            </Button>
          </div>
        )}
      </section>

      {showControls && (
        <TerminalControls
          onSendKey={onSendKey}
          onFlushComposition={onFlushComposition}
          onRefocus={onFocusTerminal}
          isKeyboardVisible={isKeyboardVisible}
        />
      )}

      <footer
        className="relative z-20 flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground"
        data-sentinel-touch-lock
        style={{ touchAction: 'none', overscrollBehaviorY: 'none' }}
      >
        <div className="min-w-0 flex-1 overflow-hidden text-[11px] text-secondary-foreground">
          <PaneStrip
            hasActiveSession={hasActiveSession}
            inspectorLoading={inspectorLoading}
            inspectorError={inspectorError}
            panes={panes}
            activeWindowIndex={activeWindowIndex}
            activePaneID={activePaneID}
            onSelectPane={onSelectPane}
            onClosePane={onClosePane}
            onRenamePane={onRenamePane}
            onSplitPaneVertical={onSplitPaneVertical}
            onSplitPaneHorizontal={onSplitPaneHorizontal}
          />
        </div>
        <div className="flex shrink-0 items-center gap-1 whitespace-nowrap">
          <button
            type="button"
            className="inline-flex h-5 w-5 items-center justify-center rounded text-secondary-foreground hover:bg-surface-active"
            onClick={onZoomOut}
            disabled={!hasActiveSession}
            aria-label="Decrease font size"
          >
            <Minus className="h-3 w-3" />
          </button>
          <button
            type="button"
            className="inline-flex h-5 w-5 items-center justify-center rounded text-secondary-foreground hover:bg-surface-active"
            onClick={onZoomIn}
            disabled={!hasActiveSession}
            aria-label="Increase font size"
          >
            <Plus className="h-3 w-3" />
          </button>
          <span className="ml-0.5">
            cols {termCols} rows {termRows}
          </span>
        </div>
      </footer>
    </main>
  )
}
