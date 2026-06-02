import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import type {
  LaunchSessionLauncherResponse,
  LaunchTmuxLauncherResponse,
  LaunchSessionPresetResponse,
  PanesResponse,
  Session,
  SessionLauncher,
  SessionLaunchersResponse,
  SessionPreset,
  SessionPresetsResponse,
  TmuxLauncher,
  TmuxLaunchersResponse,
} from '@/types'
import type { RuntimeMetrics } from '@/hooks/tmuxTypes'
import type { LauncherDraft } from '@/components/tmux/LaunchersDialog'
import AppShell from '@/components/layout/AppShell'
import SessionSidebar from '@/components/SessionSidebar'
import TmuxTerminalPanel from '@/components/TmuxTerminalPanel'
import RenameDialog from '@/components/tmux/RenameDialog'
import CreateSessionDialog from '@/components/sidebar/CreateSessionDialog'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTerminalTmux } from '@/hooks/useTerminalTmux'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { usePresence } from '@/hooks/usePresence'
import { useSeenTracking } from '@/hooks/useSeenTracking'
import { useInspector } from '@/hooks/useInspector'
import { useSessionCRUD } from '@/hooks/useSessionCRUD'
import { useTmuxEventsSocket } from '@/hooks/useTmuxEventsSocket'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import { TMUX_SESSIONS_QUERY_KEY } from '@/lib/tmuxQueryCache'
import { applySidebarOrder, moveSidebarItem, sortBySidebarOrder } from '@/lib/sessionSidebarOrder'
import { sanitizeTmuxPaneTitle, sanitizeTmuxWindowName, slugifyTmuxName } from '@/lib/tmuxName'
import { loadPersistedTabs, persistTabs, tabsReducer } from '@/tabsReducer'

const LaunchersDialog = lazy(() => import('@/components/tmux/LaunchersDialog'))

function asText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function asTimestampText(value: unknown): string {
  const text = asText(value)
  return text.startsWith('0001-01-01') ? '' : text
}

function normalizeSessionLauncher(
  rawLauncher: Partial<SessionLauncher> & Record<string, unknown>,
): SessionLauncher | null {
  const id = asText(rawLauncher.id)
  if (id.trim() === '') {
    return null
  }

  return {
    id,
    name: asText(rawLauncher.name),
    cwd: asText(rawLauncher.cwd),
    icon: asText(rawLauncher.icon),
    user: asText(rawLauncher.user),
    sortOrder:
      typeof rawLauncher.sortOrder === 'number' && Number.isFinite(rawLauncher.sortOrder)
        ? Math.trunc(rawLauncher.sortOrder)
        : undefined,
    createdAt: asTimestampText(rawLauncher.createdAt),
    updatedAt: asTimestampText(rawLauncher.updatedAt),
    lastUsedAt: asTimestampText(rawLauncher.lastUsedAt),
    useCount:
      typeof rawLauncher.useCount === 'number' && Number.isFinite(rawLauncher.useCount)
        ? Math.max(0, Math.trunc(rawLauncher.useCount))
        : 0,
  }
}

function normalizeTmuxLauncher(
  rawLauncher: Partial<TmuxLauncher> & Record<string, unknown>,
): TmuxLauncher | null {
  const id = asText(rawLauncher.id)
  if (id.trim() === '') {
    return null
  }

  const cwdMode = rawLauncher.cwdMode
  const normalizedCwdMode =
    cwdMode === 'session' || cwdMode === 'active-pane' || cwdMode === 'fixed' ? cwdMode : 'session'

  const userMode = rawLauncher.userMode
  const normalizedUserMode = userMode === 'session' || userMode === 'fixed' ? userMode : 'session'

  return {
    id,
    name: asText(rawLauncher.name),
    icon: asText(rawLauncher.icon),
    command: asText(rawLauncher.command),
    cwdMode: normalizedCwdMode,
    cwdValue: asText(rawLauncher.cwdValue),
    windowName: asText(rawLauncher.windowName),
    userMode: normalizedUserMode,
    userValue: asText(rawLauncher.userValue),
    sortOrder:
      typeof rawLauncher.sortOrder === 'number' && Number.isFinite(rawLauncher.sortOrder)
        ? Math.trunc(rawLauncher.sortOrder)
        : undefined,
    createdAt: asTimestampText(rawLauncher.createdAt),
    updatedAt: asTimestampText(rawLauncher.updatedAt),
    lastUsedAt: asTimestampText(rawLauncher.lastUsedAt),
  }
}

function TmuxPage() {
  const { tokenRequired, defaultCwd, hostname } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const queryClient = useQueryClient()

  const [launchersOpen, setLaunchersOpen] = useState(false)
  const [createSessionOpen, setCreateSessionOpen] = useState(false)
  const [launchers, setLaunchers] = useState<Array<TmuxLauncher>>([])
  const [sessionLaunchers, setSessionLaunchers] = useState<Array<SessionLauncher>>([])
  const [sessionPresets, setSessionPresets] = useState<Array<SessionPreset>>([])

  // ---- Tabs state ----
  const [tabsState, dispatchTabs] = useReducer(tabsReducer, undefined, loadPersistedTabs)
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
  }, [tabsState.activeSession])

  // ---- Sessions state ----
  const [sessions, setSessions] = useState<Array<Session>>(
    () => queryClient.getQueryData<Array<Session>>(TMUX_SESSIONS_QUERY_KEY) ?? [],
  )
  const [filter, setFilter] = useState('')
  const debouncedFilter = useDebouncedValue(filter)
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

  const refreshSessionPresets = useCallback(
    async (options?: { quiet?: boolean }) => {
      try {
        const data = await api<SessionPresetsResponse>('/api/tmux/session-presets')
        setSessionPresets(data.presets)
      } catch (error) {
        if (!options?.quiet) {
          const message =
            error instanceof Error ? error.message : 'failed to refresh pinned sessions'
          pushErrorToast('Pinned Sessions', message)
        }
      }
    },
    [api, pushErrorToast],
  )

  const refreshLaunchers = useCallback(
    async (options?: { quiet?: boolean }) => {
      try {
        const data = await api<TmuxLaunchersResponse>('/api/tmux/launchers')
        const nextLaunchers = Array.isArray(data.launchers)
          ? data.launchers.flatMap((rawLauncher) => {
              const normalized = normalizeTmuxLauncher(
                rawLauncher as Partial<TmuxLauncher> & Record<string, unknown>,
              )
              return normalized === null ? [] : [normalized]
            })
          : []
        setLaunchers(nextLaunchers)
      } catch (error) {
        if (!options?.quiet) {
          const message = error instanceof Error ? error.message : 'failed to refresh launchers'
          pushErrorToast('Launchers', message)
        }
      }
    },
    [api, pushErrorToast],
  )

  const refreshSessionLaunchers = useCallback(
    async (options?: { quiet?: boolean }) => {
      try {
        const data = await api<SessionLaunchersResponse>('/api/tmux/session-launchers')
        const nextLaunchers = Array.isArray(data.launchers)
          ? data.launchers.flatMap((rawLauncher) => {
              const normalized = normalizeSessionLauncher(
                rawLauncher as Partial<SessionLauncher> & Record<string, unknown>,
              )
              return normalized === null ? [] : [normalized]
            })
          : []
        setSessionLaunchers(nextLaunchers)
      } catch (error) {
        if (!options?.quiet) {
          const message =
            error instanceof Error ? error.message : 'failed to refresh session launchers'
          pushErrorToast('Session Launchers', message)
        }
      }
    },
    [api, pushErrorToast],
  )

  useEffect(() => {
    void refreshSessionPresets({ quiet: true })
  }, [refreshSessionPresets])

  useEffect(() => {
    void refreshLaunchers({ quiet: true })
  }, [refreshLaunchers])

  useEffect(() => {
    void refreshSessionLaunchers({ quiet: true })
  }, [refreshSessionLaunchers])

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
    reconnectActiveSession,
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
  const refreshSessionsViaRef = useCallback(async () => {
    await refreshSessionsRef.current()
  }, [])

  // ---- Inspector hook ----
  const inspector = useInspector({
    api,
    tabsStateRef,
    sessionsRef,
    runtimeMetricsRef,
    activeSession: tabsState.activeSession,
    setTmuxUnavailable,
    setSessions,
    refreshSessions: refreshSessionsViaRef,
    pushErrorToast,
    pushSuccessToast,
    setConnection,
    dispatchTabs,
    closeCurrentSocket,
    resetTerminal,
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
    clearPendingInspectorSessionState: inspector.clearPendingInspectorSessionState,
    pushErrorToast,
    pushSuccessToast,
    pendingCreateSessionsRef: inspector.pendingCreateSessionsRef,
    pendingKillSessionsRef: inspector.pendingKillSessionsRef,
    refreshSessionPresets,
  })

  // Wire the forwarding ref
  useEffect(() => {
    refreshSessionsRef.current = sessionCRUD.refreshSessions
  }, [sessionCRUD.refreshSessions])

  const saveSessionPreset = useCallback(
    async (input: {
      previousName: string
      name: string
      cwd: string
      icon: string
      user: string
    }) => {
      const existingByName = sessionPresets.find((preset) => preset.name === input.name)
      const targetName = input.previousName || existingByName?.name || ''
      const isUpdate = targetName !== ''
      const path = isUpdate
        ? `/api/tmux/session-presets/${encodeURIComponent(targetName)}`
        : '/api/tmux/session-presets'

      try {
        await api(path, {
          method: isUpdate ? 'PATCH' : 'POST',
          body: JSON.stringify({
            name: input.name,
            cwd: input.cwd,
            icon: input.icon,
            user: input.user,
          }),
        })
        await refreshSessionPresets()
        pushSuccessToast(
          'Pinned Sessions',
          isUpdate ? `pinned session "${input.name}" updated` : `session "${input.name}" pinned`,
        )
        return true
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to pin session'
        pushErrorToast('Pinned Sessions', message)
        return false
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshSessionPresets, sessionPresets],
  )

  const deleteSessionPreset = useCallback(
    async (name: string) => {
      try {
        await api<void>(`/api/tmux/session-presets/${encodeURIComponent(name)}`, {
          method: 'DELETE',
        })
        await refreshSessionPresets()
        pushSuccessToast('Pinned Sessions', `session "${name}" unpinned`)
        return true
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to unpin session'
        pushErrorToast('Pinned Sessions', message)
        return false
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshSessionPresets],
  )

  const pinSession = useCallback(
    async (sessionName: string) => {
      const session = sessions.find((item) => item.name === sessionName)
      if (!session) {
        pushErrorToast('Pinned Sessions', 'session not found')
        return
      }

      try {
        const data = await api<PanesResponse>(
          `/api/tmux/sessions/${encodeURIComponent(sessionName)}/panes`,
        )
        const cwd =
          data.panes.find((pane) => pane.active && pane.currentPath)?.currentPath ??
          data.panes.find((pane) => pane.currentPath)?.currentPath ??
          defaultCwd

        const existing = sessionPresets.some((preset) => preset.name === sessionName)
        await saveSessionPreset({
          previousName: existing ? sessionName : '',
          name: sessionName,
          cwd,
          icon: session.icon || 'terminal',
          user: session.user ?? '',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to pin session'
        pushErrorToast('Pinned Sessions', message)
      }
    },
    [api, defaultCwd, pushErrorToast, saveSessionPreset, sessionPresets, sessions],
  )

  const launchSessionPreset = useCallback(
    async (name: string) => {
      const preset = sessionPresets.find((item) => item.name === name)
      if (!preset) {
        pushErrorToast('Pinned Sessions', 'pinned session not found')
        return
      }

      try {
        const data = await api<LaunchSessionPresetResponse>(
          `/api/tmux/session-presets/${encodeURIComponent(name)}/launch`,
          {
            method: 'POST',
          },
        )
        sessionCRUD.activateSession(data.name, preset.icon)
        setConnection('connecting', `opening ${data.name}`)
        void inspector.refreshInspector(data.name)
        void sessionCRUD.refreshSessions()
        void refreshSessionPresets({ quiet: true })
        pushSuccessToast(
          'Pinned Sessions',
          data.created ? `session "${data.name}" created` : `session "${data.name}" opened`,
        )
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to launch pinned session'
        pushErrorToast('Pinned Sessions', message)
      }
    },
    [
      api,
      inspector,
      pushErrorToast,
      pushSuccessToast,
      refreshSessionPresets,
      sessionCRUD,
      sessionPresets,
      setConnection,
    ],
  )

  const saveSessionLauncher = useCallback(
    async (input: { id: string; name: string; cwd: string; icon: string; user: string }) => {
      const isUpdate = input.id.trim() !== ''
      const path = isUpdate
        ? `/api/tmux/session-launchers/${encodeURIComponent(input.id)}`
        : '/api/tmux/session-launchers'

      try {
        const data = await api<{ launcher: SessionLauncher }>(path, {
          method: isUpdate ? 'PATCH' : 'POST',
          body: JSON.stringify({
            name: input.name,
            cwd: input.cwd,
            icon: input.icon,
            user: input.user,
          }),
        })
        await refreshSessionLaunchers()
        pushSuccessToast(
          'Session Launchers',
          isUpdate
            ? `session launcher "${data.launcher.name}" updated`
            : `session launcher "${data.launcher.name}" saved`,
        )
        return data.launcher.id
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to save session launcher'
        pushErrorToast('Session Launchers', message)
        return ''
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshSessionLaunchers],
  )

  const deleteSessionLauncher = useCallback(
    async (id: string) => {
      const existing = sessionLaunchers.find((launcher) => launcher.id === id)
      try {
        await api<void>(`/api/tmux/session-launchers/${encodeURIComponent(id)}`, {
          method: 'DELETE',
        })
        await refreshSessionLaunchers()
        pushSuccessToast(
          'Session Launchers',
          existing ? `session launcher "${existing.name}" deleted` : 'session launcher deleted',
        )
        return true
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to delete session launcher'
        pushErrorToast('Session Launchers', message)
        return false
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshSessionLaunchers, sessionLaunchers],
  )

  const launchSessionLauncher = useCallback(
    async (id: string) => {
      const launcher = sessionLaunchers.find((item) => item.id === id)
      if (!launcher) {
        pushErrorToast('Session Launchers', 'session launcher not found')
        return
      }

      try {
        const data = await api<LaunchSessionLauncherResponse>(
          `/api/tmux/session-launchers/${encodeURIComponent(id)}/launch`,
          {
            method: 'POST',
          },
        )
        sessionCRUD.activateSession(data.name, launcher.icon)
        setConnection('connecting', `opening ${data.name}`)
        void inspector.refreshInspector(data.name)
        void sessionCRUD.refreshSessions()
        void refreshSessionLaunchers({ quiet: true })
        pushSuccessToast('Session Launchers', `session "${data.name}" created`)
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to launch session launcher'
        pushErrorToast('Session Launchers', message)
      }
    },
    [
      api,
      inspector,
      pushErrorToast,
      pushSuccessToast,
      refreshSessionLaunchers,
      sessionCRUD,
      sessionLaunchers,
      setConnection,
    ],
  )

  const saveLauncher = useCallback(
    async (draft: LauncherDraft) => {
      const payload = {
        name: draft.name,
        icon: draft.icon,
        command: draft.command,
        cwdMode: draft.cwdMode,
        cwdValue: draft.cwdValue,
        windowName: draft.windowName,
        userMode: draft.userMode,
        userValue: draft.userValue,
      }
      const isUpdate = Boolean(draft.id)
      const path = isUpdate
        ? `/api/tmux/launchers/${encodeURIComponent(draft.id ?? '')}`
        : '/api/tmux/launchers'

      try {
        const data = await api<{ launcher: TmuxLauncher }>(path, {
          method: isUpdate ? 'PATCH' : 'POST',
          body: JSON.stringify(payload),
        })
        await refreshLaunchers()
        pushSuccessToast(
          'Launchers',
          isUpdate
            ? `launcher "${data.launcher.name}" updated`
            : `launcher "${data.launcher.name}" created`,
        )
        return data.launcher.id
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to save launcher'
        pushErrorToast('Launchers', message)
        throw error instanceof Error ? error : new Error(message)
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshLaunchers],
  )

  const deleteLauncher = useCallback(
    async (launcherID: string) => {
      const existing = launchers.find((launcher) => launcher.id === launcherID)
      try {
        await api<void>(`/api/tmux/launchers/${encodeURIComponent(launcherID)}`, {
          method: 'DELETE',
        })
        await refreshLaunchers()
        pushSuccessToast(
          'Launchers',
          existing ? `launcher "${existing.name}" deleted` : 'launcher deleted',
        )
        return true
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to delete launcher'
        pushErrorToast('Launchers', message)
        return false
      }
    },
    [api, launchers, pushErrorToast, pushSuccessToast, refreshLaunchers],
  )

  const reorderLaunchers = useCallback(
    async (activeID: string, overID: string) => {
      const current = launchers.map((launcher) => launcher.id)
      const next = moveSidebarItem(current, activeID, overID)
      if (next === current) return

      const launchersByID = new Map(launchers.map((launcher) => [launcher.id, launcher]))
      setLaunchers(
        next.flatMap((id, index) => {
          const launcher = launchersByID.get(id)
          if (!launcher) {
            return []
          }
          return [
            {
              ...launcher,
              sortOrder: index + 1,
            },
          ]
        }),
      )

      try {
        await api<void>('/api/tmux/launchers/order', {
          method: 'PATCH',
          body: JSON.stringify({ ids: next }),
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to reorder launchers'
        pushErrorToast('Launchers', message)
        void refreshLaunchers({ quiet: true })
      }
    },
    [api, launchers, pushErrorToast, refreshLaunchers],
  )

  const launchLauncher = useCallback(
    async (launcherID: string) => {
      const activeSession = tabsStateRef.current.activeSession.trim()
      if (activeSession === '') {
        pushErrorToast('Launchers', 'attach to a session before launching')
        return
      }

      const launcher = launchers.find((item) => item.id === launcherID)
      if (!launcher) {
        pushErrorToast('Launchers', 'launcher not found')
        return
      }

      try {
        const data = await api<LaunchTmuxLauncherResponse>(
          `/api/tmux/sessions/${encodeURIComponent(activeSession)}/launchers/${encodeURIComponent(launcherID)}/launch`,
          { method: 'POST' },
        )
        inspector.setActiveWindowIndexOverride(data.windowIndex)
        inspector.setActivePaneIDOverride(data.paneId)
        void inspector.refreshInspector(activeSession, { background: true })
        void sessionCRUD.refreshSessions()
        void refreshLaunchers({ quiet: true })
        pushSuccessToast('Launchers', `launcher "${launcher.name}" opened as "${data.windowName}"`)
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to launch window'
        pushErrorToast('Launchers', message)
        void inspector.refreshInspector(activeSession, { background: true })
        void sessionCRUD.refreshSessions()
      }
    },
    [
      api,
      inspector,
      launchers,
      pushErrorToast,
      pushSuccessToast,
      refreshLaunchers,
      sessionCRUD,
      tabsStateRef,
    ],
  )

  // ---- Derived active window/pane ----
  const activeWindowIndex = useMemo(() => {
    if (inspector.activeWindowIndexOverride !== null) return inspector.activeWindowIndexOverride
    return inspector.windows.find((w) => w.active)?.index ?? null
  }, [inspector.activeWindowIndexOverride, inspector.windows])

  const activePaneID = useMemo(() => {
    if (inspector.activePaneIDOverride !== null) return inspector.activePaneIDOverride
    if (activeWindowIndex === null) return null
    const inWindow = inspector.panes.filter((p) => p.windowIndex === activeWindowIndex)
    return inWindow.find((p) => p.active)?.paneId ?? inWindow.at(0)?.paneId ?? null
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
  const { syncActivityDelta, forceReconnect: forceReconnectEvents } = useTmuxEventsSocket({
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
    pushErrorToast,
    applySessionActivityPatches: inspector.applySessionActivityPatches,
    applyInspectorProjectionPatches: inspector.applyInspectorProjectionPatches,
    settlePendingSeenAcks: seen.settlePendingSeenAcks,
    seenAckWaitersRef: seen.seenAckWaitersRef,
    handleTmuxSessionsEvent: sessionCRUD.handleTmuxSessionsEvent,
    handleTmuxInspectorEvent: inspector.handleTmuxInspectorEvent,
  })

  // ---- Resync handler ----
  const handleResync = useCallback(() => {
    const active = tabsState.activeSession.trim()
    if (active === '') return
    inspector.clearPendingInspectorSessionState(active)
    void sessionCRUD.refreshSessions()
    void inspector.refreshInspector(active)
    void syncActivityDelta({ reason: 'manual-resync', force: true })
    forceReconnectEvents()
    reconnectActiveSession({ force: true })
    pushSuccessToast('Resync', 'Session state refreshed')
  }, [
    tabsState.activeSession,
    inspector,
    sessionCRUD,
    syncActivityDelta,
    forceReconnectEvents,
    reconnectActiveSession,
    pushSuccessToast,
  ])

  // ---- Derived state ----
  const orderedSessions = useMemo(() => {
    return sortBySidebarOrder(sessions)
  }, [sessions])

  const orderedSessionPresets = useMemo(() => {
    return sortBySidebarOrder(sessionPresets)
  }, [sessionPresets])

  const orderedSessionLaunchers = useMemo(() => {
    return sortBySidebarOrder(sessionLaunchers)
  }, [sessionLaunchers])

  const recentLauncher = useMemo(() => {
    const usedLaunchers = launchers.filter((launcher) =>
      Number.isFinite(Date.parse(launcher.lastUsedAt)),
    )
    if (usedLaunchers.length === 0) return null
    return [...usedLaunchers].sort((left, right) => {
      const leftTime = left.lastUsedAt ? Date.parse(left.lastUsedAt) : 0
      const rightTime = right.lastUsedAt ? Date.parse(right.lastUsedAt) : 0
      if (leftTime !== rightTime) return rightTime - leftTime
      return (left.sortOrder ?? 0) - (right.sortOrder ?? 0)
    })[0]
  }, [launchers])

  const filteredSessions = useMemo(() => {
    const query = debouncedFilter.trim().toLowerCase()
    if (!query) return orderedSessions
    return orderedSessions.filter((s) => s.name.toLowerCase().includes(query))
  }, [debouncedFilter, orderedSessions])

  const reorderPinnedSessions = useCallback(
    async (activeName: string, overName: string) => {
      const current = orderedSessionPresets.map((preset) => preset.name)
      const next = moveSidebarItem(current, activeName, overName)
      if (next === current) {
        return
      }

      setSessionPresets((prev) => applySidebarOrder(prev, next))
      try {
        await api<void>('/api/tmux/session-presets/order', {
          method: 'PATCH',
          body: JSON.stringify({ names: next }),
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to reorder pinned sessions'
        pushErrorToast('Pinned Sessions', message)
        void refreshSessionPresets({ quiet: true })
      }
    },
    [api, orderedSessionPresets, pushErrorToast, refreshSessionPresets],
  )

  const reorderSessionLaunchers = useCallback(
    async (activeID: string, overID: string) => {
      const current = orderedSessionLaunchers.map((launcher) => launcher.id)
      const next = moveSidebarItem(current, activeID, overID)
      if (next === current) {
        return
      }

      const launchersByID = new Map(
        orderedSessionLaunchers.map((launcher) => [launcher.id, launcher]),
      )
      setSessionLaunchers(
        next.flatMap((id, index) => {
          const launcher = launchersByID.get(id)
          if (!launcher) {
            return []
          }
          return [{ ...launcher, sortOrder: index + 1 }]
        }),
      )
      try {
        await api<void>('/api/tmux/session-launchers/order', {
          method: 'PATCH',
          body: JSON.stringify({ ids: next }),
        })
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to reorder session launchers'
        pushErrorToast('Session Launchers', message)
        void refreshSessionLaunchers({ quiet: true })
      }
    },
    [api, orderedSessionLaunchers, pushErrorToast, refreshSessionLaunchers],
  )

  const reorderVisibleSessions = useCallback(
    async (activeName: string, overName: string) => {
      const pinnedNames = new Set(sessionPresets.map((preset) => preset.name))
      const current = sortBySidebarOrder(sessions)
        .filter((session) => !pinnedNames.has(session.name))
        .map((session) => session.name)
      const next = moveSidebarItem(current, activeName, overName)
      if (next === current) {
        return
      }

      setSessions((prev) => applySidebarOrder(prev, next))
      try {
        await api<void>('/api/tmux/sessions/order', {
          method: 'PATCH',
          body: JSON.stringify({ names: next }),
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'failed to reorder sessions'
        pushErrorToast('Sessions', message)
        void sessionCRUD.refreshSessions()
      }
    },
    [api, pushErrorToast, sessionCRUD, sessionPresets, sessions],
  )

  const activeSessionUser = useMemo(
    () => sessions.find((s) => s.name === tabsState.activeSession)?.user ?? '',
    [sessions, tabsState.activeSession],
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
          presets={orderedSessionPresets}
          launchers={orderedSessionLaunchers}
          filter={filter}
          tmuxUnavailable={tmuxUnavailable}
          onFilterChange={setFilter}
          onTokenChange={setToken}
          onCreate={(name, cwd, user) => sessionCRUD.createSession(name, cwd, '', user)}
          onSaveLauncher={saveSessionLauncher}
          onDeleteLauncher={deleteSessionLauncher}
          onLaunchLauncher={(id) => {
            void launchSessionLauncher(id)
          }}
          onReorderLaunchers={reorderSessionLaunchers}
          onPinSession={(name) => {
            void pinSession(name)
          }}
          onUnpinSession={deleteSessionPreset}
          onLaunchPreset={(name) => {
            void launchSessionPreset(name)
          }}
          onReorderPinned={reorderPinnedSessions}
          onReorderSession={reorderVisibleSessions}
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
        hostname={hostname}
        connectionState={connectionState}
        statusDetail={statusDetail}
        sidebarCollapsed={layout.sidebarCollapsed}
        openTabs={tabsState.openTabs}
        activeSession={tabsState.activeSession}
        sessionUser={activeSessionUser}
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
        launchers={launchers}
        recentLauncher={recentLauncher}
        onLaunchLauncher={(launcherID) => {
          void launchLauncher(launcherID)
        }}
        onOpenLaunchers={() => setLaunchersOpen(true)}
        onReorderWindow={inspector.reorderWindows}
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
        onOpenCreateSession={() => setCreateSessionOpen(true)}
        onResync={handleResync}
      />

      <Suspense fallback={null}>
        {launchersOpen && (
          <LaunchersDialog
            open={launchersOpen}
            onOpenChange={setLaunchersOpen}
            launchers={launchers}
            onSave={saveLauncher}
            onDelete={deleteLauncher}
            onReorder={(activeID, overID) => {
              void reorderLaunchers(activeID, overID)
            }}
          />
        )}

        {createSessionOpen && (
          <CreateSessionDialog
            open={createSessionOpen}
            onOpenChange={setCreateSessionOpen}
            defaultCwd={defaultCwd}
            onCreate={(name, cwd, user) => sessionCRUD.createSession(name, cwd, '', user)}
          />
        )}
      </Suspense>

      <RenameDialog
        open={sessionCRUD.renameDialogOpen}
        onOpenChange={sessionCRUD.setRenameDialogOpen}
        title="Rename session"
        description="Enter a new name for the active session."
        value={sessionCRUD.renameValue}
        onValueChange={(value) => sessionCRUD.setRenameValue(slugifyTmuxName(value))}
        onSubmit={sessionCRUD.handleSubmitRename}
        onClose={() => sessionCRUD.setRenameSessionTarget(null)}
      />

      <RenameDialog
        open={inspector.renameWindowDialogOpen}
        onOpenChange={inspector.setRenameWindowDialogOpen}
        title="Rename window"
        description="Enter a new name for this tmux window."
        value={inspector.renameWindowValue}
        onValueChange={(value) => inspector.setRenameWindowValue(sanitizeTmuxWindowName(value))}
        onSubmit={inspector.handleSubmitRenameWindow}
        onClose={() => inspector.setRenameWindowTarget(null)}
      />

      <RenameDialog
        open={inspector.renamePaneDialogOpen}
        onOpenChange={inspector.setRenamePaneDialogOpen}
        title="Rename pane"
        description="Enter a new title for this tmux pane."
        value={inspector.renamePaneValue}
        onValueChange={(value) => inspector.setRenamePaneValue(sanitizeTmuxPaneTitle(value))}
        onSubmit={inspector.handleSubmitRenamePane}
        onClose={() => inspector.setRenamePaneTarget(null)}
      />
    </AppShell>
  )
}

export const Route = createFileRoute('/tmux')({
  component: TmuxPage,
})
