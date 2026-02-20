import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import type { Session } from '@/types'
import type { RuntimeMetrics } from '@/hooks/tmuxTypes'
import AppShell from '@/components/layout/AppShell'
import SessionSidebar from '@/components/SessionSidebar'
import TmuxTerminalPanel from '@/components/TmuxTerminalPanel'
import CreateSessionDialog from '@/components/sidebar/CreateSessionDialog'
import GuardrailsDialog from '@/components/tmux/GuardrailsDialog'
import TimelineDialog from '@/components/tmux/TimelineDialog'
import RecoveryDialog from '@/components/tmux/RecoveryDialog'
import GuardrailConfirmDialog from '@/components/GuardrailConfirmDialog'
import RenameDialog from '@/components/tmux/RenameDialog'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTerminalTmux } from '@/hooks/useTerminalTmux'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { usePresence } from '@/hooks/usePresence'
import { useSeenTracking } from '@/hooks/useSeenTracking'
import { useRecovery } from '@/hooks/useRecovery'
import { useTmuxTimeline } from '@/hooks/useTmuxTimeline'
import { useInspector } from '@/hooks/useInspector'
import { useSessionCRUD } from '@/hooks/useSessionCRUD'
import { useTmuxEventsSocket } from '@/hooks/useTmuxEventsSocket'
import { TMUX_SESSIONS_QUERY_KEY } from '@/lib/tmuxQueryCache'
import { loadPersistedTabs, persistTabs, tabsReducer } from '@/tabsReducer'

function TmuxPage() {
  const { tokenRequired, defaultCwd } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const queryClient = useQueryClient()

  // ---- Guardrails dialog state ----
  const [guardrailsOpen, setGuardrailsOpen] = useState(false)
  const [createSessionOpen, setCreateSessionOpen] = useState(false)

  // ---- Guardrail confirm state (shared across session CRUD + inspector) ----
  const [guardrailConfirm, setGuardrailConfirm] = useState<{
    ruleName: string
    message: string
    onConfirm: () => void
  } | null>(null)

  const requestGuardrailConfirm = useCallback(
    (ruleName: string, message: string, onConfirm: () => void) => {
      setGuardrailConfirm({ ruleName, message, onConfirm })
    },
    [],
  )

  // ---- Tabs state ----
  const [tabsState, dispatchTabs] = useReducer(
    tabsReducer,
    undefined,
    loadPersistedTabs,
  )
  useEffect(() => {
    persistTabs(tabsState)
  }, [tabsState])
  const restoredRef = useRef(false)
  useEffect(() => {
    if (restoredRef.current) return
    restoredRef.current = true
    if (tabsState.activeSession !== '') {
      dispatchTabs({ type: 'activate', session: tabsState.activeSession })
    }
  }, [])

  // ---- Sessions state ----
  const [sessions, setSessions] = useState<Array<Session>>(
    () =>
      queryClient.getQueryData<Array<Session>>(TMUX_SESSIONS_QUERY_KEY) ?? [],
  )
  const [filter, setFilter] = useState('')
  const [tmuxUnavailable, setTmuxUnavailable] = useState(false)

  // ---- Shared refs (created here, passed to hooks) ----
  const tabsStateRef = useRef(tabsState)
  const sessionsRef = useRef<Array<Session>>([])
  const presenceSocketRef = useRef<WebSocket | null>(null)
  const eventsSocketConnectedRef = useRef(false)
  const runtimeMetricsRef = useRef<RuntimeMetrics>({
    wsMessages: 0,
    wsReconnects: 0,
    wsOpenCount: 0,
    wsCloseCount: 0,
    sessionsRefreshCount: 0,
    inspectorRefreshCount: 0,
    recoveryRefreshCount: 0,
    fallbackRefreshCount: 0,
    deltaSyncCount: 0,
    deltaSyncErrors: 0,
    deltaOverflowCount: 0,
  })

  // Sync shared refs
  useEffect(() => {
    tabsStateRef.current = tabsState
  }, [tabsState])
  useEffect(() => {
    sessionsRef.current = sessions
    queryClient.setQueryData(TMUX_SESSIONS_QUERY_KEY, sessions)
  }, [queryClient, sessions])

  // ---- API ----
  const api = useTmuxApi()

  // ---- Toast helpers ----
  const pushErrorToast = useCallback(
    (title: string, message: string) => {
      pushToast({ level: 'error', title, message })
    },
    [pushToast],
  )
  const pushSuccessToast = useCallback(
    (title: string, message: string) => {
      pushToast({ level: 'success', title, message })
    },
    [pushToast],
  )

  // ---- Terminal hook ----
  const handleAttachedMobile = useCallback(() => {
    layout.setSidebarOpen(false)
  }, [layout])

  const {
    getTerminalHostRef,
    connectionState,
    statusDetail,
    termCols,
    termRows,
    setConnection,
    closeCurrentSocket,
    resetTerminal,
    sendKey,
    flushComposition,
    focusTerminal,
    zoomIn,
    zoomOut,
  } = useTerminalTmux({
    openTabs: tabsState.openTabs,
    activeSession: tabsState.activeSession,
    activeEpoch: tabsState.activeEpoch,
    sidebarCollapsed: layout.sidebarCollapsed,
    onAttachedMobile: handleAttachedMobile,
    allowWheelInAlternateBuffer: true,
    suppressBrowserContextMenu: true,
  })

  // ---- Forwarding ref for refreshSessions (resolves circular dependency) ----
  const refreshSessionsRef = useRef<() => Promise<void>>(async () => {})

  // ---- Inspector hook ----
  const inspector = useInspector({
    api,
    tabsStateRef,
    sessionsRef,
    runtimeMetricsRef,
    activeSession: tabsState.activeSession,
    setTmuxUnavailable,
    setSessions,
    refreshSessions: async () => {
      await refreshSessionsRef.current()
    },
    eventsSocketConnectedRef,
    pushErrorToast,
    pushSuccessToast,
    setConnection,
    requestGuardrailConfirm,
  })

  // ---- Session CRUD hook ----
  const sessionCRUD = useSessionCRUD({
    api,
    tabsStateRef,
    sessionsRef,
    runtimeMetricsRef,
    dispatchTabs,
    setSessions,
    setTmuxUnavailable,
    closeCurrentSocket,
    resetTerminal,
    setConnection,
    connectionState,
    refreshInspector: inspector.refreshInspector,
    clearPendingInspectorSessionState:
      inspector.clearPendingInspectorSessionState,
    pushErrorToast,
    pushSuccessToast,
    pendingCreateSessionsRef: inspector.pendingCreateSessionsRef,
    requestGuardrailConfirm,
  })

  // Wire the forwarding ref
  useEffect(() => {
    refreshSessionsRef.current = sessionCRUD.refreshSessions
  }, [sessionCRUD.refreshSessions])

  // ---- Recovery hook ----
  const recovery = useRecovery({
    api,
    runtimeMetricsRef,
    refreshSessions: sessionCRUD.refreshSessions,
    pushErrorToast,
    pushSuccessToast,
  })

  // ---- Timeline hook ----
  const timeline = useTmuxTimeline({
    api,
    activeSession: tabsState.activeSession,
  })

  // ---- Derived active window/pane ----
  const activeWindowIndex = useMemo(() => {
    if (inspector.activeWindowIndexOverride !== null)
      return inspector.activeWindowIndexOverride
    return inspector.windows.find((w) => w.active)?.index ?? null
  }, [inspector.activeWindowIndexOverride, inspector.windows])

  const activePaneID = useMemo(() => {
    if (inspector.activePaneIDOverride !== null)
      return inspector.activePaneIDOverride
    if (activeWindowIndex === null) return null
    const inWindow = inspector.panes.filter(
      (p) => p.windowIndex === activeWindowIndex,
    )
    return (
      inWindow.find((p) => p.active)?.paneId ?? inWindow.at(0)?.paneId ?? null
    )
  }, [inspector.activePaneIDOverride, activeWindowIndex, inspector.panes])

  // ---- Seen tracking hook ----
  const seen = useSeenTracking({
    api,
    presenceSocketRef,
    activeSession: tabsState.activeSession,
    activeWindowIndex,
    activePaneID,
    applySessionActivityPatches: inspector.applySessionActivityPatches,
    applyInspectorProjectionPatches: inspector.applyInspectorProjectionPatches,
  })

  // ---- Presence hook ----
  const presence = usePresence({
    api,
    presenceSocketRef,
    tabsStateRef,
    activeWindowIndex,
    activePaneID,
    activeSession: tabsState.activeSession,
  })

  // ---- Events socket hook ----
  useTmuxEventsSocket({
    api,
    authenticated,
    tokenRequired,
    setToken,
    presenceSocketRef,
    tabsStateRef,
    eventsSocketConnectedRef,
    runtimeMetricsRef,
    lastSessionsRefreshAtRef: sessionCRUD.lastSessionsRefreshAtRef,
    sendPresenceOverWS: presence.sendPresenceOverWS,
    refreshSessions: sessionCRUD.refreshSessions,
    refreshInspector: inspector.refreshInspector,
    refreshRecovery: recovery.refreshRecovery,
    pushErrorToast,
    applySessionActivityPatches: inspector.applySessionActivityPatches,
    applyInspectorProjectionPatches: inspector.applyInspectorProjectionPatches,
    settlePendingSeenAcks: seen.settlePendingSeenAcks,
    seenAckWaitersRef: seen.seenAckWaitersRef,
    timelineOpenRef: timeline.timelineOpenRef,
    timelineSessionFilterRef: timeline.timelineSessionFilterRef,
    loadTimelineRef: timeline.loadTimelineRef,
  })

  // ---- Derived state ----
  const orderedSessions = useMemo(() => {
    const list = [...sessions]
    list.sort((left, right) =>
      left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }),
    )
    return list
  }, [sessions])

  const filteredSessions = useMemo(() => {
    const query = filter.trim().toLowerCase()
    if (!query) return orderedSessions
    return orderedSessions.filter((s) => s.name.toLowerCase().includes(query))
  }, [filter, orderedSessions])

  const timelineSessionOptions = useMemo(
    () => orderedSessions.map((item) => item.name),
    [orderedSessions],
  )

  // ---- JSX ----
  return (
    <AppShell
      sidebar={
        <SessionSidebar
          sessions={filteredSessions}
          totalSessions={sessions.length}
          openTabs={tabsState.openTabs}
          activeSession={tabsState.activeSession}
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          defaultCwd={defaultCwd}
          filter={filter}
          tmuxUnavailable={tmuxUnavailable}
          onFilterChange={setFilter}
          onTokenChange={setToken}
          onCreate={(name, cwd) => {
            void sessionCRUD.createSession(name, cwd)
          }}
          onAttach={sessionCRUD.activateSession}
          onRename={sessionCRUD.handleOpenRenameDialogForSession}
          onDetach={sessionCRUD.detachSession}
          onKill={(name) => {
            void sessionCRUD.killSession(name)
          }}
          onChangeIcon={sessionCRUD.setSessionIcon}
        />
      }
    >
      <TmuxTerminalPanel
        connectionState={connectionState}
        statusDetail={statusDetail}
        openTabs={tabsState.openTabs}
        activeSession={tabsState.activeSession}
        inspectorLoading={inspector.inspectorLoading}
        inspectorError={inspector.inspectorError}
        windows={inspector.windows}
        panes={inspector.panes}
        activeWindowIndex={activeWindowIndex}
        activePaneID={activePaneID}
        termCols={termCols}
        termRows={termRows}
        getTerminalHostRef={getTerminalHostRef}
        onToggleSidebarOpen={() => layout.setSidebarOpen((prev) => !prev)}
        onSelectWindow={inspector.selectWindow}
        onSelectPane={inspector.selectPane}
        onRenameWindow={inspector.handleOpenRenameWindow}
        onRenamePane={inspector.handleOpenRenamePane}
        onCreateWindow={inspector.createWindow}
        onCloseWindow={inspector.closeWindow}
        onSplitPaneVertical={() => inspector.splitPane('vertical')}
        onSplitPaneHorizontal={() => inspector.splitPane('horizontal')}
        onClosePane={inspector.closePane}
        onRenameTab={sessionCRUD.handleOpenRenameDialogForSession}
        onKillTab={(name) => {
          void sessionCRUD.killSession(name)
        }}
        onSelectTab={sessionCRUD.activateSession}
        onCloseTab={sessionCRUD.closeTab}
        onReorderTabs={sessionCRUD.reorderTabs}
        onSendKey={sendKey}
        onFlushComposition={flushComposition}
        onFocusTerminal={focusTerminal}
        onZoomIn={zoomIn}
        onZoomOut={zoomOut}
        onOpenGuardrails={() => setGuardrailsOpen(true)}
        onOpenSnapshots={() => recovery.setRecoveryDialogOpen(true)}
        onOpenTimeline={() => {
          timeline.setTimelineOpen(true)
          void timeline.loadTimeline({ quiet: true })
        }}
        onOpenCreateSession={() => setCreateSessionOpen(true)}
      />

      <GuardrailsDialog
        open={guardrailsOpen}
        onOpenChange={setGuardrailsOpen}
      />

      <CreateSessionDialog
        open={createSessionOpen}
        onOpenChange={setCreateSessionOpen}
        defaultCwd={defaultCwd}
        onCreate={(name, cwd) => {
          void sessionCRUD.createSession(name, cwd)
        }}
      />

      <TimelineDialog
        open={timeline.timelineOpen}
        onOpenChange={timeline.setTimelineOpen}
        loading={timeline.timelineLoading}
        error={timeline.timelineError}
        events={timeline.timelineEvents}
        hasMore={timeline.timelineHasMore}
        query={timeline.timelineQuery}
        severity={timeline.timelineSeverity}
        eventType={timeline.timelineEventType}
        sessionFilter={timeline.timelineSessionFilter}
        sessionOptions={timelineSessionOptions}
        onQueryChange={timeline.setTimelineQuery}
        onSeverityChange={timeline.setTimelineSeverity}
        onEventTypeChange={timeline.setTimelineEventType}
        onSessionFilterChange={timeline.setTimelineSessionFilter}
        onRefresh={() => {
          void timeline.loadTimeline()
        }}
      />

      <RecoveryDialog
        open={recovery.recoveryDialogOpen}
        onOpenChange={recovery.setRecoveryDialogOpen}
        recoverySessions={recovery.recoverySessions}
        recoveryJobs={recovery.recoveryJobs}
        recoverySnapshots={recovery.recoverySnapshots}
        selectedRecoverySession={recovery.selectedRecoverySession}
        selectedSnapshotID={recovery.selectedSnapshotID}
        selectedSnapshot={recovery.selectedSnapshot}
        recoveryLoading={recovery.recoveryLoading}
        recoveryBusy={recovery.recoveryBusy}
        recoveryError={recovery.recoveryError}
        restoreMode={recovery.restoreMode}
        restoreConflictPolicy={recovery.restoreConflictPolicy}
        restoreTargetSession={recovery.restoreTargetSession}
        onRefresh={() => {
          void recovery.refreshRecovery()
        }}
        onSelectSession={(session) => {
          recovery.setSelectedRecoverySession(session)
          recovery.setRestoreTargetSessionRaw(session)
        }}
        onSelectSnapshot={(id) => {
          void recovery.loadRecoverySnapshot(id)
        }}
        onRestoreModeChange={recovery.setRestoreMode}
        onConflictPolicyChange={recovery.setRestoreConflictPolicy}
        onTargetSessionChange={recovery.setRestoreTargetSession}
        onRestore={() => {
          void recovery.restoreSelectedSnapshot()
        }}
        onArchive={(session) => {
          void recovery.archiveRecoverySession(session)
        }}
      />

      <GuardrailConfirmDialog
        open={guardrailConfirm !== null}
        ruleName={guardrailConfirm?.ruleName ?? ''}
        message={guardrailConfirm?.message ?? ''}
        onOpenChange={() => setGuardrailConfirm(null)}
        onConfirm={() => {
          guardrailConfirm?.onConfirm()
          setGuardrailConfirm(null)
        }}
      />

      <RenameDialog
        open={sessionCRUD.renameDialogOpen}
        onOpenChange={sessionCRUD.setRenameDialogOpen}
        title="Rename session"
        description="Enter a new name for the active session."
        value={sessionCRUD.renameValue}
        onValueChange={sessionCRUD.setRenameValue}
        onSubmit={sessionCRUD.handleSubmitRename}
        onClose={() => sessionCRUD.setRenameSessionTarget(null)}
      />

      <RenameDialog
        open={inspector.renameWindowDialogOpen}
        onOpenChange={inspector.setRenameWindowDialogOpen}
        title="Rename window"
        description="Enter a new name for this tmux window."
        value={inspector.renameWindowValue}
        onValueChange={inspector.setRenameWindowValue}
        onSubmit={inspector.handleSubmitRenameWindow}
        onClose={() => inspector.setRenameWindowIndex(null)}
      />

      <RenameDialog
        open={inspector.renamePaneDialogOpen}
        onOpenChange={inspector.setRenamePaneDialogOpen}
        title="Rename pane"
        description="Enter a new title for this tmux pane."
        value={inspector.renamePaneValue}
        onValueChange={inspector.setRenamePaneValue}
        onSubmit={inspector.handleSubmitRenamePane}
        onClose={() => inspector.setRenamePaneID(null)}
      />
    </AppShell>
  )
}

export const Route = createFileRoute('/tmux')({
  component: TmuxPage,
})
