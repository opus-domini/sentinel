import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, Plus, RefreshCw } from 'lucide-react'
import type { MutableRefObject } from 'react'
import type { TerminalsResponse } from '@/types'
import AppShell from '@/components/layout/AppShell'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TooltipHelper } from '@/components/TooltipHelper'
import ConnectionBadge from '@/components/ConnectionBadge'
import HelpDialog from '@/components/HelpDialog'
import SessionTabs from '@/components/SessionTabs'
import SystemTerminalDetail from '@/components/SystemTerminalDetail'
import TerminalsSidebar from '@/components/TerminalsSidebar'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { useTerminalTmux } from '@/hooks/useTerminalTmux'
import { initialTabsState, tabsReducer } from '@/tabsReducer'

function nextTerminalName(
  existing: ReadonlyArray<string>,
  indexRef: MutableRefObject<number>,
): string {
  const taken = new Set(existing)
  let index = Math.max(1, indexRef.current)
  for (;;) {
    const candidate = `terminal-${index}`
    index += 1
    if (!taken.has(candidate)) {
      indexRef.current = index
      return candidate
    }
  }
}

function TerminalsPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()

  const [tabsState, dispatchTabs] = useReducer(tabsReducer, initialTabsState)
  const [selectedSystemTTY, setSelectedSystemTTY] = useState<string | null>(
    null,
  )
  const [systemTerminals, setSystemTerminals] = useState<
    TerminalsResponse['terminals']
  >([])
  const [loadingSystemTerminals, setLoadingSystemTerminals] = useState(false)
  const [systemTerminalsError, setSystemTerminalsError] = useState('')

  const tabsStateRef = useRef(tabsState)
  const terminalIndexRef = useRef(1)
  const api = useTmuxApi(token)

  useEffect(() => {
    tabsStateRef.current = tabsState
  }, [tabsState])

  const {
    getTerminalHostRef,
    connectionState,
    statusDetail,
    termCols,
    termRows,
    closeCurrentSocket,
  } = useTerminalTmux({
    openTabs: tabsState.openTabs,
    activeSession: tabsState.activeSession,
    activeEpoch: tabsState.activeEpoch,
    token,
    sidebarCollapsed: layout.sidebarCollapsed,
    onAttachedMobile: () => {
      layout.setSidebarOpen(false)
    },
    wsPath: '/ws/terminals',
    wsQueryKey: 'terminal',
    connectingVerb: 'starting',
    connectedVerb: 'connected',
  })

  const refreshSystemTerminals = useCallback(
    async (background = false) => {
      if (!background) setLoadingSystemTerminals(true)
      try {
        const data = await api<TerminalsResponse>('/api/terminals')
        setSystemTerminals(data.terminals)
        setSystemTerminalsError('')
      } catch (error) {
        setSystemTerminalsError(
          error instanceof Error
            ? error.message
            : 'failed to load system terminals',
        )
      } finally {
        if (!background) setLoadingSystemTerminals(false)
      }
    },
    [api],
  )

  useEffect(() => {
    void refreshSystemTerminals()
    const id = window.setInterval(() => {
      void refreshSystemTerminals(true)
    }, 4_000)
    return () => {
      window.clearInterval(id)
    }
  }, [refreshSystemTerminals])

  const openTerminal = useCallback(() => {
    const name = nextTerminalName(
      tabsStateRef.current.openTabs,
      terminalIndexRef,
    )
    dispatchTabs({ type: 'activate', session: name })
  }, [])

  const closeTerminal = useCallback(
    (terminalName: string) => {
      if (terminalName === tabsStateRef.current.activeSession)
        closeCurrentSocket('terminal closed')
      dispatchTabs({ type: 'close', session: terminalName })
    },
    [closeCurrentSocket],
  )

  const selectTerminal = useCallback(
    (terminalName: string) => {
      setSelectedSystemTTY(null)
      dispatchTabs({ type: 'activate', session: terminalName })
      layout.setSidebarOpen(false)
    },
    [layout],
  )

  const selectSystemTerminal = useCallback(
    (tty: string) => {
      setSelectedSystemTTY(tty)
      layout.setSidebarOpen(false)
    },
    [layout],
  )

  const reorderTabs = useCallback((from: number, to: number) => {
    dispatchTabs({ type: 'reorder', from, to })
  }, [])

  useEffect(() => {
    if (connectionState !== 'error') return
    pushToast({
      level: 'error',
      title: 'Terminal connection',
      message: statusDetail,
    })
  }, [connectionState, pushToast, statusDetail])

  return (
    <AppShell
      sidebar={
        <TerminalsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          sentinelTerminals={tabsState.openTabs}
          activeTerminal={tabsState.activeSession}
          systemTerminals={systemTerminals}
          loadingSystemTerminals={loadingSystemTerminals}
          systemTerminalsError={systemTerminalsError}
          activeSystemTTY={selectedSystemTTY}
          onTokenChange={setToken}
          onCreateTerminal={openTerminal}
          onSelectTerminal={selectTerminal}
          onCloseTerminal={closeTerminal}
          onSelectSystemTerminal={selectSystemTerminal}
        />
      }
    >
      <main className="grid min-w-0 grid-cols-1 grid-rows-[40px_30px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(30,64,175,.18),transparent_34%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <span className="truncate">Sentinel</span>
            <span className="text-muted-foreground">/</span>
            <span className="truncate text-muted-foreground">terminals</span>
          </div>
          <div className="flex items-center gap-1.5">
            <ConnectionBadge state={connectionState} />
            <HelpDialog context="terminal" />
            <TooltipHelper content="Refresh system terminals">
              <Button
                variant="outline"
                size="icon"
                onClick={() => {
                  void refreshSystemTerminals()
                }}
                aria-label="Refresh system terminals"
              >
                <RefreshCw className="h-4 w-4" />
              </Button>
            </TooltipHelper>
          </div>
        </header>

        <SessionTabs
          openTabs={tabsState.openTabs}
          activeSession={tabsState.activeSession}
          onSelect={selectTerminal}
          onClose={closeTerminal}
          onReorder={reorderTabs}
          emptyLabel="No open terminals"
        />

        <section className="min-h-0">
          {selectedSystemTTY != null ? (
            <div className="h-full min-h-0 border-x border-border-subtle bg-surface-inset md:border-x-0">
              <SystemTerminalDetail
                tty={selectedSystemTTY}
                onBack={() => setSelectedSystemTTY(null)}
              />
            </div>
          ) : (
            <div className="relative h-full min-h-0 border-x border-border-subtle md:border-x-0">
              {tabsState.openTabs.length === 0 && (
                <EmptyState className="border-dashed text-[12px]">
                  <div className="flex flex-col items-center gap-2">
                    <p>Create a terminal to start interacting.</p>
                    <Button
                      variant="outline"
                      size="default"
                      className="border-primary/40 bg-primary/15 text-primary-text-bright hover:bg-primary/25"
                      onClick={openTerminal}
                    >
                      <Plus className="h-4 w-4" />
                      New terminal
                    </Button>
                  </div>
                </EmptyState>
              )}

              {tabsState.openTabs.map((terminalName) => (
                <div
                  key={terminalName}
                  ref={getTerminalHostRef(terminalName)}
                  className={
                    terminalName === tabsState.activeSession
                      ? 'absolute inset-0 min-h-0 overflow-hidden'
                      : 'hidden absolute inset-0 min-h-0 overflow-hidden'
                  }
                />
              ))}
            </div>
          )}
        </section>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">{statusDetail}</span>
          <span className="shrink-0 whitespace-nowrap">
            {selectedSystemTTY != null
              ? `tty ${selectedSystemTTY}`
              : `cols ${termCols} rows ${termRows}`}
          </span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/terminals')({
  component: TerminalsPage,
})
