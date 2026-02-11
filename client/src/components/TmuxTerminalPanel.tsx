import { Menu, Minus, Plus } from 'lucide-react'
import { useCallback } from 'react'
import ConnectionBadge from './ConnectionBadge'
import HelpDialog from './HelpDialog'
import SessionTabs from './SessionTabs'
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
  tmuxUnavailable: boolean
  tmuxUnavailableMessage: string
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
}

export default function TmuxTerminalPanel({
  connectionState,
  statusDetail,
  tmuxUnavailable,
  tmuxUnavailableMessage,
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
}: TmuxTerminalPanelProps) {
  const isMobileLayout = useIsMobileLayout()
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

  return (
    <main
      className={cn(
        'grid min-h-0 min-w-0 grid-cols-1 overflow-hidden bg-[radial-gradient(circle_at_20%_-10%,rgba(30,64,175,.18),transparent_34%),var(--background)]',
        showControls && tmuxUnavailable && 'grid-rows-[40px_30px_auto_1fr_auto_28px]',
        showControls &&
          !tmuxUnavailable &&
          'grid-rows-[40px_30px_1fr_auto_28px]',
        !showControls && tmuxUnavailable && 'grid-rows-[40px_30px_auto_1fr_28px]',
        !showControls && !tmuxUnavailable && 'grid-rows-[40px_30px_1fr_28px]',
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
          <HelpDialog
            context="tmux"
            triggerClassName="relative top-px size-5 cursor-pointer rounded-sm p-0 text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground"
            iconClassName="size-3"
          />
        </div>
        <div className="flex items-center gap-1.5">
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

      {tmuxUnavailable && (
        <div className="border-x border-b border-warning/45 bg-warning/20 px-3 py-2 text-[12px] text-warning-foreground">
          <p className="font-semibold">tmux is not installed on this host.</p>
          <p className="mt-0.5 text-secondary-foreground">
            Install tmux and restart Sentinel to enable session management.
          </p>
          {tmuxUnavailableMessage.trim() !== '' && (
            <p className="mt-1 truncate text-[11px] text-secondary-foreground">
              {tmuxUnavailableMessage}
            </p>
          )}
        </div>
      )}

      <section className="grid min-h-0 min-w-0 grid-cols-1 grid-rows-[36px_1fr] border-x border-border-subtle">
        <div className="min-w-0 overflow-hidden border-b border-border-subtle bg-surface-overlay px-2.5 py-1">
          <WindowStrip
            hasActiveSession={hasActiveSession}
            inspectorLoading={inspectorLoading}
            inspectorError={inspectorError}
            windows={windows}
            activeWindowIndex={activeWindowIndex}
            onSelectWindow={onSelectWindow}
            onCloseWindow={onCloseWindow}
            onRenameWindow={onRenameWindow}
            onCreateWindow={onCreateWindow}
          />
        </div>

        <div className="relative min-h-0">
          <TerminalHost
            openTabs={openTabs}
            activeSession={activeSession}
            getTerminalHostRef={getTerminalHostRef}
          />
        </div>
      </section>

      {showControls && (
        <TerminalControls
          onSendKey={onSendKey}
          onFlushComposition={onFlushComposition}
          onRefocus={onFocusTerminal}
          isKeyboardVisible={isKeyboardVisible}
        />
      )}

      <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
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
