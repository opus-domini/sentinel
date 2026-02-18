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
import type {
  PaneInfo,
  PanesResponse,
  RecoveryJob,
  RecoveryJobResponse,
  RecoveryOverviewResponse,
  RecoverySession,
  RecoverySnapshotResponse,
  RecoverySnapshotView,
  RecoverySnapshotsResponse,
  Session,
  SessionsResponse,
  TimelineEvent,
  TimelineResponse,
  WindowInfo,
  WindowsResponse,
} from '@/types'
import type { TmuxInspectorSnapshot } from '@/lib/tmuxQueryCache'
import type {
  SessionActivityPatch,
  SessionPatchApplyResult,
} from '@/lib/tmuxSessionEvents'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import AppShell from '@/components/layout/AppShell'
import SessionSidebar from '@/components/SessionSidebar'
import TmuxTerminalPanel from '@/components/TmuxTerminalPanel'
import TimelineDialog from '@/components/tmux/TimelineDialog'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTerminalTmux } from '@/hooks/useTerminalTmux'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { slugifyTmuxName } from '@/lib/tmuxName'
import { shouldRefreshSessionsFromEvent } from '@/lib/tmuxSessionEvents'
import { shouldSkipInspectorRefresh } from '@/lib/tmuxInspectorRefresh'
import {
  TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
  TMUX_SESSIONS_QUERY_KEY,
  shouldCacheActiveInspectorSnapshot,
  tmuxInspectorQueryKey,
  tmuxTimelineQueryKey,
} from '@/lib/tmuxQueryCache'
import {
  buildTimelineQueryString,
  shouldRefreshTimelineFromEvent,
} from '@/lib/tmuxTimeline'
import {
  mergePendingCreateSessions,
  upsertOptimisticAttachedSession,
} from '@/lib/tmuxSessionCreate'
import {
  addPendingPaneClose,
  addPendingWindowClose,
  addPendingWindowCreate,
  buildPendingSplitPaneID,
  clearPendingPaneClosesForSession,
  clearPendingWindowClosesForSession,
  clearPendingWindowCreatesForSession,
  clearPendingWindowPaneFloor,
  clearPendingWindowPaneFloorsForSession,
  isPendingSplitPaneID,
  mergePendingInspectorSnapshot,
  removePendingPaneClose,
  removePendingWindowClose,
  removePendingWindowCreate,
  setPendingWindowPaneFloor,
} from '@/lib/tmuxInspectorOptimistic'
import { randomId } from '@/lib/utils'
import { buildWSProtocols } from '@/lib/wsAuth'
import { loadPersistedTabs, persistTabs, tabsReducer } from '@/tabsReducer'

function isTmuxBinaryMissingMessage(message: string): boolean {
  const normalized = message.trim().toLowerCase()
  return normalized.includes('tmux binary not found')
}

function resolvePresenceTerminalID(): string {
  const key = 'sentinel.tmux.presence.terminalId'
  const fromStorage = window.sessionStorage.getItem(key)
  if (fromStorage && fromStorage.trim() !== '') {
    return fromStorage
  }

  const generated = randomId()
  window.sessionStorage.setItem(key, generated)
  return generated
}

function sameWindowProjection(
  left: Array<WindowInfo>,
  right: Array<WindowInfo>,
): boolean {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i]
    const b = right[i]
    if (
      a.session !== b.session ||
      a.index !== b.index ||
      a.name !== b.name ||
      a.active !== b.active ||
      a.panes !== b.panes ||
      (a.unreadPanes ?? 0) !== (b.unreadPanes ?? 0) ||
      (a.hasUnread ?? false) !== (b.hasUnread ?? false) ||
      (a.rev ?? 0) !== (b.rev ?? 0) ||
      (a.activityAt ?? '') !== (b.activityAt ?? '')
    ) {
      return false
    }
  }
  return true
}

function samePaneProjection(
  left: Array<PaneInfo>,
  right: Array<PaneInfo>,
): boolean {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i]
    const b = right[i]
    if (
      a.session !== b.session ||
      a.windowIndex !== b.windowIndex ||
      a.paneIndex !== b.paneIndex ||
      a.paneId !== b.paneId ||
      a.title !== b.title ||
      a.active !== b.active ||
      a.tty !== b.tty ||
      (a.currentPath ?? '') !== (b.currentPath ?? '') ||
      (a.startCommand ?? '') !== (b.startCommand ?? '') ||
      (a.currentCommand ?? '') !== (b.currentCommand ?? '') ||
      (a.tailPreview ?? '') !== (b.tailPreview ?? '') ||
      (a.revision ?? 0) !== (b.revision ?? 0) ||
      (a.seenRevision ?? 0) !== (b.seenRevision ?? 0) ||
      (a.hasUnread ?? false) !== (b.hasUnread ?? false) ||
      (a.changedAt ?? '') !== (b.changedAt ?? '')
    ) {
      return false
    }
  }
  return true
}

type SeenAckMessage = {
  eventId?: number
  type?: string
  requestId?: string
  globalRev?: number
  sessionPatches?: Array<SessionActivityPatch>
  inspectorPatches?: Array<InspectorSessionPatch>
}

type SeenCommandPayload = {
  session: string
  scope: 'pane' | 'window' | 'session'
  paneId?: string
  windowIndex?: number
}

type InspectorWindowPatch = {
  session?: string
  index?: number
  name?: string
  active?: boolean
  panes?: number
  unreadPanes?: number
  hasUnread?: boolean
  rev?: number
  activityAt?: string
}

type InspectorPanePatch = {
  session?: string
  windowIndex?: number
  paneIndex?: number
  paneId?: string
  title?: string
  active?: boolean
  tty?: string
  currentPath?: string
  startCommand?: string
  currentCommand?: string
  tailPreview?: string
  revision?: number
  seenRevision?: number
  hasUnread?: boolean
  changedAt?: string
}

type InspectorSessionPatch = {
  session?: string
  windows?: Array<InspectorWindowPatch>
  panes?: Array<InspectorPanePatch>
}

type ActivityDeltaChange = {
  id?: number
  globalRev?: number
  entityType?: string
  session?: string
  windowIndex?: number
  paneId?: string
  changeKind?: string
  changedAt?: string
}

type ActivityDeltaResponse = {
  since?: number
  limit?: number
  globalRev?: number
  overflow?: boolean
  changes?: Array<ActivityDeltaChange>
  sessionPatches?: Array<SessionActivityPatch>
  inspectorPatches?: Array<InspectorSessionPatch>
}

type RecoveryOverviewCache = {
  sessions: Array<RecoverySession>
  jobs: Array<RecoveryJob>
}

type TmuxTimelineCache = {
  events: Array<TimelineEvent>
  hasMore: boolean
}

function TmuxPage() {
  const { tokenRequired, defaultCwd } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const queryClient = useQueryClient()

  const [tabsState, rawDispatchTabs] = useReducer(
    tabsReducer,
    undefined,
    loadPersistedTabs,
  )
  const [sessions, setSessions] = useState<Array<Session>>(
    () =>
      queryClient.getQueryData<Array<Session>>(TMUX_SESSIONS_QUERY_KEY) ?? [],
  )
  const dispatchTabs = useCallback(
    (action: Parameters<typeof rawDispatchTabs>[0]) => {
      rawDispatchTabs(action)
    },
    [],
  )
  useEffect(() => {
    persistTabs(tabsState)
  }, [tabsState])
  // Re-activate persisted session after mount so the terminal hook
  // sees an epoch bump and connects the WebSocket / xterm instance.
  const restoredRef = useRef(false)
  useEffect(() => {
    if (restoredRef.current) return
    restoredRef.current = true
    if (tabsState.activeSession !== '') {
      rawDispatchTabs({ type: 'activate', session: tabsState.activeSession })
    }
  }, [])
  const [filter, setFilter] = useState('')

  const [windows, setWindows] = useState<Array<WindowInfo>>(() => {
    const active = tabsState.activeSession.trim()
    if (active === '') return []
    return (
      queryClient.getQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(active),
      )?.windows ?? []
    )
  })
  const [panes, setPanes] = useState<Array<PaneInfo>>(() => {
    const active = tabsState.activeSession.trim()
    if (active === '') return []
    return (
      queryClient.getQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(active),
      )?.panes ?? []
    )
  })
  const [activeWindowIndexOverride, setActiveWindowIndexOverride] = useState<
    number | null
  >(null)
  const [activePaneIDOverride, setActivePaneIDOverride] = useState<
    string | null
  >(null)
  const [inspectorLoading, setInspectorLoading] = useState(false)
  const [inspectorError, setInspectorError] = useState('')
  const [tmuxUnavailable, setTmuxUnavailable] = useState(false)

  const [killDialogSession, setKillDialogSession] = useState<string | null>(
    null,
  )
  const [renameDialogOpen, setRenameDialogOpen] = useState(false)
  const [renameSessionTarget, setRenameSessionTarget] = useState<string | null>(
    null,
  )
  const [renameValue, setRenameValue] = useState('')
  const [renameWindowDialogOpen, setRenameWindowDialogOpen] = useState(false)
  const [renameWindowIndex, setRenameWindowIndex] = useState<number | null>(
    null,
  )
  const [renameWindowValue, setRenameWindowValue] = useState('')
  const [renamePaneDialogOpen, setRenamePaneDialogOpen] = useState(false)
  const [renamePaneID, setRenamePaneID] = useState<string | null>(null)
  const [renamePaneValue, setRenamePaneValue] = useState('')
  const [recoverySessions, setRecoverySessions] = useState<
    Array<RecoverySession>
  >(
    () =>
      queryClient.getQueryData<RecoveryOverviewCache>(
        TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      )?.sessions ?? [],
  )
  const [recoveryJobs, setRecoveryJobs] = useState<Array<RecoveryJob>>(
    () =>
      queryClient.getQueryData<RecoveryOverviewCache>(
        TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      )?.jobs ?? [],
  )
  const [recoveryDialogOpen, setRecoveryDialogOpen] = useState(false)
  const [recoverySnapshots, setRecoverySnapshots] = useState<
    Array<{ id: number; capturedAt: string; windows: number; panes: number }>
  >([])
  const [selectedRecoverySession, setSelectedRecoverySession] = useState<
    string | null
  >(null)
  const [selectedSnapshotID, setSelectedSnapshotID] = useState<number | null>(
    null,
  )
  const [selectedSnapshot, setSelectedSnapshot] =
    useState<RecoverySnapshotView | null>(null)
  const [recoveryLoading, setRecoveryLoading] = useState(false)
  const [recoveryBusy, setRecoveryBusy] = useState(false)
  const [recoveryError, setRecoveryError] = useState('')
  const [restoreMode, setRestoreMode] = useState<'safe' | 'confirm' | 'full'>(
    'confirm',
  )
  const [eventsSocketConnected, setEventsSocketConnected] = useState(false)
  const [restoreConflictPolicy, setRestoreConflictPolicy] = useState<
    'rename' | 'replace' | 'skip'
  >('rename')
  const [restoreTargetSession, setRestoreTargetSession] = useState('')
  const [timelineOpen, setTimelineOpen] = useState(false)
  const [timelineEvents, setTimelineEvents] = useState<Array<TimelineEvent>>(
    () =>
      queryClient.getQueryData<TmuxTimelineCache>(
        tmuxTimelineQueryKey({
          session: '',
          query: '',
          severity: 'all',
          eventType: 'all',
          limit: 180,
        }),
      )?.events ?? [],
  )
  const [timelineHasMore, setTimelineHasMore] = useState(
    () =>
      queryClient.getQueryData<TmuxTimelineCache>(
        tmuxTimelineQueryKey({
          session: '',
          query: '',
          severity: 'all',
          eventType: 'all',
          limit: 180,
        }),
      )?.hasMore ?? false,
  )
  const [timelineLoading, setTimelineLoading] = useState(false)
  const [timelineError, setTimelineError] = useState('')
  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')
  const [timelineEventType, setTimelineEventType] = useState('all')
  const [timelineSessionFilter, setTimelineSessionFilter] = useState('active')

  const api = useTmuxApi(token)
  const refreshGenerationRef = useRef(0)
  const inspectorGenerationRef = useRef(0)
  const recoveryGenerationRef = useRef(0)
  const timelineGenerationRef = useRef(0)
  const pendingCreateSessionsRef = useRef(new Map<string, string>())
  const pendingKillSessionsRef = useRef(new Set<string>())
  const pendingRenameSessionsRef = useRef(new Map<string, string>())
  const pendingCreateWindowsRef = useRef(new Map<string, Set<number>>())
  const pendingCloseWindowsRef = useRef(new Map<string, Set<number>>())
  const pendingClosePanesRef = useRef(new Map<string, Set<string>>())
  const pendingWindowPaneFloorsRef = useRef(
    new Map<string, Map<number, number>>(),
  )
  const sessionsRef = useRef<Array<Session>>([])
  const windowsRef = useRef<Array<WindowInfo>>([])
  const panesRef = useRef<Array<PaneInfo>>([])
  const inspectorLoadingRef = useRef(false)
  const tabsStateRef = useRef(tabsState)
  const seenAckKeyRef = useRef('')
  const seenRequestSeqRef = useRef(0)
  const seenAckWaitersRef = useRef(new Map<string, (ok: boolean) => void>())
  const presenceTerminalIDRef = useRef('')
  const presenceSocketRef = useRef<WebSocket | null>(null)
  const presenceLastSignatureRef = useRef('')
  const presenceLastSentAtRef = useRef(0)
  const presenceHTTPInFlightRef = useRef(false)
  const activeWindowOverrideRef = useRef<number | null>(null)
  const activePaneOverrideRef = useRef<string | null>(null)
  const activeWindowIndexRef = useRef<number | null>(null)
  const activePaneIDRef = useRef<string | null>(null)
  const eventsSocketConnectedRef = useRef(false)
  const timelineOpenRef = useRef(false)
  const timelineSessionFilterRef = useRef('active')
  const loadTimelineRef = useRef<(options?: { quiet?: boolean }) => void>(
    () => {
      return
    },
  )
  const lastSessionsRefreshAtRef = useRef(0)
  const lastGlobalRevRef = useRef(0)
  const lastEventIDRef = useRef(0)
  const lastDeltaSyncAtRef = useRef(0)
  const deltaSyncInFlightRef = useRef(false)
  const wsReconnectAttemptsRef = useRef(0)
  const runtimeMetricsRef = useRef({
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
  const refreshTimerRef = useRef<{
    sessions: number | null
    inspector: number | null
    recovery: number | null
    timeline: number | null
  }>({ sessions: null, inspector: null, recovery: null, timeline: null })

  const bumpRuntimeMetric = useCallback(
    (key: keyof typeof runtimeMetricsRef.current, delta = 1): number => {
      const current = runtimeMetricsRef.current[key]
      const next = current + delta
      runtimeMetricsRef.current[key] = next
      return next
    },
    [],
  )

  useEffect(() => {
    ;(
      window as typeof window & { __SENTINEL_TMUX_METRICS?: unknown }
    ).__SENTINEL_TMUX_METRICS = runtimeMetricsRef.current
    return () => {
      ;(
        window as typeof window & { __SENTINEL_TMUX_METRICS?: unknown }
      ).__SENTINEL_TMUX_METRICS = undefined
    }
  }, [])

  useEffect(() => {
    tabsStateRef.current = tabsState
  }, [tabsState])
  useEffect(() => {
    sessionsRef.current = sessions
    queryClient.setQueryData(TMUX_SESSIONS_QUERY_KEY, sessions)
  }, [queryClient, sessions])
  useEffect(() => {
    windowsRef.current = windows
  }, [windows])
  useEffect(() => {
    panesRef.current = panes
  }, [panes])
  useEffect(() => {
    inspectorLoadingRef.current = inspectorLoading
  }, [inspectorLoading])
  useEffect(() => {
    queryClient.setQueryData<RecoveryOverviewCache>(
      TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      {
        sessions: recoverySessions,
        jobs: recoveryJobs,
      },
    )
  }, [queryClient, recoveryJobs, recoverySessions])
  useEffect(() => {
    const rawScope = timelineSessionFilter.trim()
    const session =
      rawScope === '' || rawScope === 'all'
        ? ''
        : rawScope === 'active'
          ? tabsState.activeSession.trim()
          : rawScope
    queryClient.setQueryData<TmuxTimelineCache>(
      tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      }),
      {
        events: timelineEvents,
        hasMore: timelineHasMore,
      },
    )
  }, [
    queryClient,
    tabsState.activeSession,
    timelineEventType,
    timelineEvents,
    timelineHasMore,
    timelineQuery,
    timelineSessionFilter,
    timelineSeverity,
  ])
  useEffect(() => {
    const active = tabsState.activeSession.trim()
    if (!shouldCacheActiveInspectorSnapshot(active, windows, panes)) {
      return
    }
    queryClient.setQueryData<TmuxInspectorSnapshot>(
      tmuxInspectorQueryKey(active),
      {
        windows,
        panes,
      },
    )
  }, [panes, queryClient, tabsState.activeSession, windows])
  useEffect(() => {
    presenceTerminalIDRef.current = resolvePresenceTerminalID()
  }, [])
  useEffect(() => {
    const active = tabsState.activeSession.trim()
    if (active === '') {
      setActiveWindowIndexOverride(null)
      setActivePaneIDOverride(null)
      return
    }
    const cached = queryClient.getQueryData<TmuxInspectorSnapshot>(
      tmuxInspectorQueryKey(active),
    )
    if (cached) {
      setWindows((prev) =>
        sameWindowProjection(prev, cached.windows) ? prev : cached.windows,
      )
      setPanes((prev) =>
        samePaneProjection(prev, cached.panes) ? prev : cached.panes,
      )
      setInspectorError('')
      setInspectorLoading(false)
      inspectorLoadingRef.current = false
    } else {
      setWindows([])
      setPanes([])
    }
    setActiveWindowIndexOverride(null)
    setActivePaneIDOverride(null)
  }, [queryClient, tabsState.activeSession])
  useEffect(() => {
    activeWindowOverrideRef.current = activeWindowIndexOverride
  }, [activeWindowIndexOverride])
  useEffect(() => {
    activePaneOverrideRef.current = activePaneIDOverride
  }, [activePaneIDOverride])

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
    token,
    sidebarCollapsed: layout.sidebarCollapsed,
    onAttachedMobile: handleAttachedMobile,
    allowWheelInAlternateBuffer: true,
    suppressBrowserContextMenu: true,
  })

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

  const activeWindowIndex = useMemo(() => {
    if (activeWindowIndexOverride !== null) return activeWindowIndexOverride
    return windows.find((w) => w.active)?.index ?? null
  }, [activeWindowIndexOverride, windows])

  const activePaneID = useMemo(() => {
    if (activePaneIDOverride !== null) return activePaneIDOverride
    if (activeWindowIndex === null) return null
    const inWindow = panes.filter((p) => p.windowIndex === activeWindowIndex)
    return (
      inWindow.find((p) => p.active)?.paneId ?? inWindow.at(0)?.paneId ?? null
    )
  }, [activePaneIDOverride, activeWindowIndex, panes])

  useEffect(() => {
    activeWindowIndexRef.current = activeWindowIndex
    activePaneIDRef.current = activePaneID
  }, [activePaneID, activeWindowIndex])
  useEffect(() => {
    eventsSocketConnectedRef.current = eventsSocketConnected
  }, [eventsSocketConnected])
  useEffect(() => {
    timelineOpenRef.current = timelineOpen
  }, [timelineOpen])
  useEffect(() => {
    timelineSessionFilterRef.current = timelineSessionFilter
  }, [timelineSessionFilter])

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

  const activateSession = useCallback((session: string) => {
    setSessions((prev) =>
      prev.map((item) =>
        item.name === session
          ? {
              ...item,
              attached: Math.max(1, item.attached),
              activityAt: new Date().toISOString(),
            }
          : item,
      ),
    )
    dispatchTabs({ type: 'activate', session })
  }, [])

  const clearPendingInspectorSessionState = useCallback((session: string) => {
    clearPendingWindowCreatesForSession(
      pendingCreateWindowsRef.current,
      session,
    )
    clearPendingWindowClosesForSession(pendingCloseWindowsRef.current, session)
    clearPendingPaneClosesForSession(pendingClosePanesRef.current, session)
    clearPendingWindowPaneFloorsForSession(
      pendingWindowPaneFloorsRef.current,
      session,
    )
  }, [])

  const clearPendingSessionRenamesForName = useCallback((session: string) => {
    const name = session.trim()
    if (name === '') return
    for (const [from, to] of pendingRenameSessionsRef.current) {
      if (from === name || to === name) {
        pendingRenameSessionsRef.current.delete(from)
      }
    }
  }, [])

  const mergeInspectorSnapshotWithPending = useCallback(
    (
      session: string,
      sourceWindows: Array<WindowInfo>,
      sourcePanes: Array<PaneInfo>,
    ) => {
      const merged = mergePendingInspectorSnapshot(
        session,
        sourceWindows,
        sourcePanes,
        {
          pendingWindowCreates: pendingCreateWindowsRef.current,
          pendingWindowCloses: pendingCloseWindowsRef.current,
          pendingPaneCloses: pendingClosePanesRef.current,
          pendingWindowPaneFloors: pendingWindowPaneFloorsRef.current,
          optimisticVisibleWindowBaseline: Math.max(
            0,
            windowsRef.current.filter(
              (windowInfo) => windowInfo.session === session,
            ).length -
              (pendingCreateWindowsRef.current.get(session)?.size ?? 0),
          ),
        },
      )
      for (const index of merged.confirmedWindowCreates) {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          session,
          index,
        )
      }
      for (const index of merged.confirmedWindowCloses) {
        removePendingWindowClose(pendingCloseWindowsRef.current, session, index)
      }
      for (const paneID of merged.confirmedPaneCloses) {
        removePendingPaneClose(pendingClosePanesRef.current, session, paneID)
      }
      for (const index of merged.confirmedWindowPaneFloors) {
        clearPendingWindowPaneFloor(
          pendingWindowPaneFloorsRef.current,
          session,
          index,
        )
      }
      return merged
    },
    [],
  )

  const refreshSessions = useCallback(async () => {
    bumpRuntimeMetric('sessionsRefreshCount')
    const gen = ++refreshGenerationRef.current
    try {
      const data = await api<SessionsResponse>('/api/tmux/sessions')
      if (gen !== refreshGenerationRef.current) return
      setTmuxUnavailable(false)
      const merged = mergePendingCreateSessions(
        data.sessions,
        pendingCreateSessionsRef.current,
        pendingKillSessionsRef.current,
        pendingRenameSessionsRef.current,
      )
      for (const name of merged.confirmedPendingNames) {
        pendingCreateSessionsRef.current.delete(name)
      }
      for (const name of merged.confirmedKilledNames) {
        pendingKillSessionsRef.current.delete(name)
      }
      for (const name of merged.confirmedRenamedNames) {
        pendingRenameSessionsRef.current.delete(name)
      }
      setSessions(merged.sessions)
      const sessionNames = merged.sessionNamesForSync
      for (const name of merged.confirmedKilledNames) {
        clearPendingSessionRenamesForName(name)
        clearPendingInspectorSessionState(name)
      }
      const cur = tabsStateRef.current.activeSession
      if (cur !== '' && !sessionNames.includes(cur)) {
        closeCurrentSocket('active session removed')
        resetTerminal()
        setConnection('disconnected', 'active session removed')
      }
      dispatchTabs({ type: 'sync', sessions: sessionNames })
      if (cur !== '' && merged.confirmedPendingNames.includes(cur)) {
        dispatchTabs({ type: 'activate', session: cur })
      }
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'failed to refresh sessions'
      const unavailable = isTmuxBinaryMissingMessage(message)
      setTmuxUnavailable(unavailable)
      setConnection('error', message)
    } finally {
      if (gen === refreshGenerationRef.current) {
        lastSessionsRefreshAtRef.current = Date.now()
      }
    }
  }, [
    api,
    bumpRuntimeMetric,
    clearPendingInspectorSessionState,
    clearPendingSessionRenamesForName,
    closeCurrentSocket,
    resetTerminal,
    setConnection,
  ])

  const applySessionActivityPatches = useCallback(
    (
      rawPatches: Array<SessionActivityPatch> | undefined,
    ): SessionPatchApplyResult => {
      if (!Array.isArray(rawPatches) || rawPatches.length === 0) {
        return {
          hasInputPatches: false,
          applied: false,
          hasUnknownSession: false,
        }
      }

      const knownSessions = new Set(
        sessionsRef.current.map((item) => item.name.trim()),
      )
      const trackedSessions = new Set<string>()
      const activeSession = tabsStateRef.current.activeSession.trim()
      if (activeSession !== '') {
        trackedSessions.add(activeSession)
      }
      for (const tab of tabsStateRef.current.openTabs) {
        const name = tab.trim()
        if (name !== '') {
          trackedSessions.add(name)
        }
      }

      let hasInputPatches = false
      let hasUnknownSession = false
      const patchesByName = new Map<string, SessionActivityPatch>()
      for (const patch of rawPatches) {
        const name = patch.name?.trim() ?? ''
        if (name === '') continue
        hasInputPatches = true
        if (!trackedSessions.has(name)) {
          continue
        }
        if (!knownSessions.has(name)) {
          hasUnknownSession = true
          continue
        }
        patchesByName.set(name, patch)
      }
      if (patchesByName.size === 0) {
        return { hasInputPatches, applied: false, hasUnknownSession }
      }

      const asNonNegativeInt = (value: number | undefined, fallback: number) =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0
          ? Math.trunc(value)
          : fallback
      const asNonNegativeInt64 = (
        value: number | undefined,
        fallback: number,
      ) =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0
          ? Math.trunc(value)
          : fallback

      setSessions((prev) =>
        prev.map((item) => {
          const patch = patchesByName.get(item.name)
          if (!patch) return item

          const activityAt =
            typeof patch.activityAt === 'string' &&
            patch.activityAt.trim() !== ''
              ? patch.activityAt
              : item.activityAt
          const lastContent =
            typeof patch.lastContent === 'string'
              ? patch.lastContent
              : item.lastContent
          const next = {
            ...item,
            attached: asNonNegativeInt(patch.attached, item.attached),
            windows: asNonNegativeInt(patch.windows, item.windows),
            panes: asNonNegativeInt(patch.panes, item.panes),
            activityAt,
            lastContent,
            unreadWindows: asNonNegativeInt(
              patch.unreadWindows,
              item.unreadWindows ?? 0,
            ),
            unreadPanes: asNonNegativeInt(
              patch.unreadPanes,
              item.unreadPanes ?? 0,
            ),
            rev: asNonNegativeInt64(patch.rev, item.rev ?? 0),
          }
          if (
            next.attached === item.attached &&
            next.windows === item.windows &&
            next.panes === item.panes &&
            next.activityAt === item.activityAt &&
            next.lastContent === item.lastContent &&
            next.unreadWindows === (item.unreadWindows ?? 0) &&
            next.unreadPanes === (item.unreadPanes ?? 0) &&
            next.rev === (item.rev ?? 0)
          ) {
            return item
          }
          return next
        }),
      )

      return { hasInputPatches, applied: true, hasUnknownSession }
    },
    [],
  )

  const applyInspectorProjectionPatches = useCallback(
    (rawPatches: Array<InspectorSessionPatch> | undefined): boolean => {
      if (!Array.isArray(rawPatches) || rawPatches.length === 0) {
        return false
      }

      const activeSession = tabsStateRef.current.activeSession.trim()
      if (activeSession === '') {
        return false
      }

      const targetPatch = rawPatches.find(
        (patch) => (patch.session?.trim() ?? '') === activeSession,
      )
      if (!targetPatch) {
        return false
      }

      const asNonNegativeInt = (value: number | undefined, fallback: number) =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0
          ? Math.trunc(value)
          : fallback
      const asNonNegativeInt64 = (
        value: number | undefined,
        fallback: number,
      ) =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0
          ? Math.trunc(value)
          : fallback

      let nextWindows: Array<WindowInfo> | null = null
      if (Array.isArray(targetPatch.windows)) {
        const parsedWindows: Array<WindowInfo> = []
        for (const rawWindow of targetPatch.windows) {
          const session = (rawWindow.session?.trim() ?? '') || activeSession
          if (session !== activeSession) continue
          const index = rawWindow.index
          if (
            typeof index !== 'number' ||
            !Number.isFinite(index) ||
            index < 0
          ) {
            continue
          }

          const unreadPanes = asNonNegativeInt(rawWindow.unreadPanes, 0)
          parsedWindows.push({
            session,
            index: Math.trunc(index),
            name: rawWindow.name ?? '',
            active: rawWindow.active === true,
            panes: asNonNegativeInt(rawWindow.panes, 0),
            unreadPanes,
            hasUnread:
              typeof rawWindow.hasUnread === 'boolean'
                ? rawWindow.hasUnread
                : unreadPanes > 0,
            rev: asNonNegativeInt64(rawWindow.rev, 0),
            activityAt:
              typeof rawWindow.activityAt === 'string'
                ? rawWindow.activityAt
                : undefined,
          })
        }
        parsedWindows.sort((left, right) => left.index - right.index)
        nextWindows = parsedWindows
      }

      let nextPanes: Array<PaneInfo> | null = null
      if (Array.isArray(targetPatch.panes)) {
        const parsedPanes: Array<PaneInfo> = []
        for (const rawPane of targetPatch.panes) {
          const session = (rawPane.session?.trim() ?? '') || activeSession
          if (session !== activeSession) continue
          const windowIndex = rawPane.windowIndex
          const paneIndex = rawPane.paneIndex
          const paneId = rawPane.paneId?.trim() ?? ''
          if (
            typeof windowIndex !== 'number' ||
            !Number.isFinite(windowIndex) ||
            windowIndex < 0 ||
            typeof paneIndex !== 'number' ||
            !Number.isFinite(paneIndex) ||
            paneIndex < 0 ||
            paneId === ''
          ) {
            continue
          }

          const revision = asNonNegativeInt64(rawPane.revision, 0)
          const seenRevision = asNonNegativeInt64(rawPane.seenRevision, 0)
          parsedPanes.push({
            session,
            windowIndex: Math.trunc(windowIndex),
            paneIndex: Math.trunc(paneIndex),
            paneId,
            title: rawPane.title ?? '',
            active: rawPane.active === true,
            tty: rawPane.tty ?? '',
            currentPath: rawPane.currentPath ?? '',
            startCommand: rawPane.startCommand ?? '',
            currentCommand: rawPane.currentCommand ?? '',
            tailPreview:
              typeof rawPane.tailPreview === 'string'
                ? rawPane.tailPreview
                : undefined,
            revision,
            seenRevision,
            hasUnread:
              typeof rawPane.hasUnread === 'boolean'
                ? rawPane.hasUnread
                : revision > seenRevision,
            changedAt:
              typeof rawPane.changedAt === 'string'
                ? rawPane.changedAt
                : undefined,
          })
        }
        parsedPanes.sort((left, right) => {
          if (left.windowIndex !== right.windowIndex) {
            return left.windowIndex - right.windowIndex
          }
          return left.paneIndex - right.paneIndex
        })
        nextPanes = parsedPanes
      }

      if (nextWindows === null && nextPanes === null) {
        return false
      }

      const merged = mergeInspectorSnapshotWithPending(
        activeSession,
        nextWindows ?? windowsRef.current,
        nextPanes ?? panesRef.current,
      )
      queryClient.setQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(activeSession),
        {
          windows: merged.windows,
          panes: merged.panes,
        },
      )

      setWindows((prev) =>
        sameWindowProjection(prev, merged.windows) ? prev : merged.windows,
      )
      setPanes((prev) =>
        samePaneProjection(prev, merged.panes) ? prev : merged.panes,
      )

      const windowOverride = activeWindowOverrideRef.current
      if (
        windowOverride !== null &&
        !merged.windows.some(
          (windowInfo) => windowInfo.index === windowOverride,
        )
      ) {
        setActiveWindowIndexOverride(null)
      }

      const paneOverride = activePaneOverrideRef.current
      if (
        paneOverride !== null &&
        !merged.panes.some((paneInfo) => paneInfo.paneId === paneOverride)
      ) {
        setActivePaneIDOverride(null)
      }

      return true
    },
    [mergeInspectorSnapshotWithPending, queryClient],
  )

  const settlePendingSeenAcks = useCallback((ok: boolean) => {
    if (seenAckWaitersRef.current.size === 0) {
      return
    }
    const pending = Array.from(seenAckWaitersRef.current.values())
    seenAckWaitersRef.current.clear()
    for (const settle of pending) {
      settle(ok)
    }
  }, [])

  const sendSeenOverWS = useCallback((payload: SeenCommandPayload) => {
    const socket = presenceSocketRef.current
    if (socket === null || socket.readyState !== WebSocket.OPEN) {
      return Promise.resolve(false)
    }

    seenRequestSeqRef.current += 1
    const requestId = `seen-${Date.now()}-${seenRequestSeqRef.current}`
    return new Promise<boolean>((resolve) => {
      let settled = false
      const settle = (ok: boolean) => {
        if (settled) return
        settled = true
        window.clearTimeout(timeoutID)
        seenAckWaitersRef.current.delete(requestId)
        resolve(ok)
      }
      const timeoutID = window.setTimeout(() => {
        settle(false)
      }, 800)
      seenAckWaitersRef.current.set(requestId, settle)
      try {
        socket.send(
          JSON.stringify({
            type: 'seen',
            requestId,
            ...payload,
          }),
        )
      } catch {
        settle(false)
      }
    })
  }, [])

  const refreshInspector = useCallback(
    async (target: string, options?: { background?: boolean }) => {
      bumpRuntimeMetric('inspectorRefreshCount')
      const session = target.trim()
      const bg = options?.background === true
      if (session === '') {
        setWindows([])
        setPanes([])
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
        setInspectorError('')
        setInspectorLoading(false)
        inspectorLoadingRef.current = false
        return
      }
      if (shouldSkipInspectorRefresh(bg, inspectorLoadingRef.current)) {
        return
      }
      const gen = ++inspectorGenerationRef.current
      if (!bg) {
        inspectorLoadingRef.current = true
        setInspectorLoading(true)
      }
      setInspectorError('')
      try {
        const windowsResponse = await api<WindowsResponse>(
          `/api/tmux/sessions/${encodeURIComponent(session)}/windows`,
        )
        if (gen !== inspectorGenerationRef.current) return
        const panesResponse = await api<PanesResponse>(
          `/api/tmux/sessions/${encodeURIComponent(session)}/panes`,
        )
        if (gen !== inspectorGenerationRef.current) return
        const merged = mergeInspectorSnapshotWithPending(
          session,
          windowsResponse.windows,
          panesResponse.panes,
        )
        queryClient.setQueryData<TmuxInspectorSnapshot>(
          tmuxInspectorQueryKey(session),
          {
            windows: merged.windows,
            panes: merged.panes,
          },
        )
        setWindows((prev) =>
          sameWindowProjection(prev, merged.windows) ? prev : merged.windows,
        )
        setPanes((prev) =>
          samePaneProjection(prev, merged.panes) ? prev : merged.panes,
        )

        const windowOverride = activeWindowOverrideRef.current
        const paneOverride = activePaneOverrideRef.current
        const fetchedActiveWindow =
          merged.windows.find((windowInfo) => windowInfo.active)?.index ?? null
        const fetchedActivePane =
          merged.panes.find((paneInfo) => paneInfo.active)?.paneId ?? null

        let keepWindowOverride =
          windowOverride !== null &&
          fetchedActiveWindow !== windowOverride &&
          merged.windows.some(
            (windowInfo) => windowInfo.index === windowOverride,
          )
        const keepPaneOverride =
          paneOverride !== null &&
          fetchedActivePane !== paneOverride &&
          merged.panes.some((paneInfo) => paneInfo.paneId === paneOverride)
        if (keepPaneOverride) {
          keepWindowOverride = true
        }

        if (!keepWindowOverride) {
          setActiveWindowIndexOverride(null)
        }
        if (!keepPaneOverride) {
          setActivePaneIDOverride(null)
        }
      } catch (error) {
        if (gen !== inspectorGenerationRef.current) return
        if (pendingCreateSessionsRef.current.has(session)) {
          // During optimistic session create, inspector fetches can race with
          // backend provisioning and return "session not found". Ignore until
          // create flow resolves and retries.
          return
        }
        const message =
          error instanceof Error
            ? error.message
            : 'failed to load session details'
        const unavailable = isTmuxBinaryMissingMessage(message)
        if (unavailable) {
          setTmuxUnavailable(true)
        }
        setInspectorError(message)
      } finally {
        if (gen === inspectorGenerationRef.current && !bg) {
          inspectorLoadingRef.current = false
          setInspectorLoading(false)
        }
      }
    },
    [api, bumpRuntimeMetric, mergeInspectorSnapshotWithPending, queryClient],
  )

  const markSeen = useCallback(
    async (params: {
      session: string
      scope: 'pane' | 'window' | 'session'
      paneId?: string
      windowIndex?: number
    }) => {
      const session = params.session.trim()
      if (session === '') return

      const body: {
        scope: 'pane' | 'window' | 'session'
        paneId?: string
        windowIndex?: number
      } = { scope: params.scope }
      if (params.scope === 'pane' && params.paneId) {
        body.paneId = params.paneId
      }
      if (params.scope === 'window' && Number.isInteger(params.windowIndex)) {
        body.windowIndex = params.windowIndex
      }

      try {
        if (
          await sendSeenOverWS({
            session,
            ...body,
          })
        ) {
          return
        }
      } catch {
        // Seen WS ack is best-effort.
      }

      try {
        const response = await api<{
          acked: boolean
          sessionPatches?: Array<SessionActivityPatch>
          inspectorPatches?: Array<InspectorSessionPatch>
        }>(`/api/tmux/sessions/${encodeURIComponent(session)}/seen`, {
          method: 'POST',
          body: JSON.stringify(body),
        })
        applySessionActivityPatches(response.sessionPatches)
        applyInspectorProjectionPatches(response.inspectorPatches)
      } catch {
        // Seen HTTP fallback is best-effort.
      }
    },
    [
      api,
      applyInspectorProjectionPatches,
      applySessionActivityPatches,
      sendSeenOverWS,
    ],
  )

  const buildPresencePayload = useCallback(() => {
    return {
      terminalId: presenceTerminalIDRef.current.trim(),
      session: tabsStateRef.current.activeSession.trim(),
      windowIndex: activeWindowIndexRef.current ?? -1,
      paneId: activePaneIDRef.current ?? '',
      visible: document.visibilityState === 'visible',
      focused: document.hasFocus(),
    }
  }, [])

  const canEmitPresence = useCallback((signature: string, force: boolean) => {
    if (force) return true
    if (signature !== presenceLastSignatureRef.current) return true
    return Date.now() - presenceLastSentAtRef.current >= 10_000
  }, [])

  const markPresenceSent = useCallback((signature: string) => {
    presenceLastSignatureRef.current = signature
    presenceLastSentAtRef.current = Date.now()
  }, [])

  const sendPresenceOverWS = useCallback(
    (force = false): boolean => {
      const socket = presenceSocketRef.current
      if (socket === null || socket.readyState !== WebSocket.OPEN) {
        return false
      }

      const payload = buildPresencePayload()
      if (payload.terminalId === '') return false

      const signature = JSON.stringify(payload)
      if (!canEmitPresence(signature, force)) {
        return true
      }

      try {
        socket.send(
          JSON.stringify({
            type: 'presence',
            ...payload,
          }),
        )
        markPresenceSent(signature)
        return true
      } catch {
        return false
      }
    },
    [buildPresencePayload, canEmitPresence, markPresenceSent],
  )

  const sendPresenceOverHTTP = useCallback(
    async (force = false) => {
      const payload = buildPresencePayload()
      if (payload.terminalId === '') return

      const signature = JSON.stringify(payload)
      if (!canEmitPresence(signature, force)) return
      if (presenceHTTPInFlightRef.current) return

      presenceHTTPInFlightRef.current = true
      try {
        await api<{ accepted: boolean }>('/api/tmux/presence', {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
        markPresenceSent(signature)
      } catch {
        // Presence fallback is best-effort.
      } finally {
        presenceHTTPInFlightRef.current = false
      }
    },
    [api, buildPresencePayload, canEmitPresence, markPresenceSent],
  )

  useEffect(() => {
    // Keep inspector in sync when user switches active session/tab.
    void refreshInspector(tabsState.activeSession)
  }, [refreshInspector, tabsState.activeSession])

  useEffect(() => {
    const session = tabsState.activeSession.trim()
    if (session === '') {
      seenAckKeyRef.current = ''
      return
    }

    if (activePaneID && activePaneID.trim() !== '') {
      const key = `${session}|pane|${activePaneID}`
      if (seenAckKeyRef.current !== key) {
        seenAckKeyRef.current = key
        void markSeen({ session, scope: 'pane', paneId: activePaneID })
      }
      return
    }

    if (activeWindowIndex !== null && activeWindowIndex >= 0) {
      const key = `${session}|window|${activeWindowIndex}`
      if (seenAckKeyRef.current !== key) {
        seenAckKeyRef.current = key
        void markSeen({
          session,
          scope: 'window',
          windowIndex: activeWindowIndex,
        })
      }
      return
    }

    const key = `${session}|session`
    if (seenAckKeyRef.current !== key) {
      seenAckKeyRef.current = key
      void markSeen({ session, scope: 'session' })
    }
  }, [activePaneID, activeWindowIndex, markSeen, tabsState.activeSession])

  useEffect(() => {
    const tick = () => {
      if (sendPresenceOverWS(false)) return
      void sendPresenceOverHTTP(false)
    }

    tick()
    const heartbeatID = window.setInterval(tick, 10_000)
    return () => {
      window.clearInterval(heartbeatID)
    }
  }, [sendPresenceOverHTTP, sendPresenceOverWS])

  useEffect(() => {
    const onPresenceSignal = () => {
      if (sendPresenceOverWS(true)) return
      void sendPresenceOverHTTP(true)
    }
    document.addEventListener('visibilitychange', onPresenceSignal)
    window.addEventListener('focus', onPresenceSignal)
    window.addEventListener('blur', onPresenceSignal)

    return () => {
      document.removeEventListener('visibilitychange', onPresenceSignal)
      window.removeEventListener('focus', onPresenceSignal)
      window.removeEventListener('blur', onPresenceSignal)
    }
  }, [sendPresenceOverHTTP, sendPresenceOverWS])

  useEffect(() => {
    if (sendPresenceOverWS(true)) return
    void sendPresenceOverHTTP(true)
  }, [
    activePaneID,
    activeWindowIndex,
    sendPresenceOverHTTP,
    sendPresenceOverWS,
    tabsState.activeSession,
  ])

  const refreshRecovery = useCallback(
    async (options?: { quiet?: boolean }) => {
      bumpRuntimeMetric('recoveryRefreshCount')
      const gen = ++recoveryGenerationRef.current
      if (!options?.quiet) {
        setRecoveryLoading(true)
      }
      try {
        const data = await api<RecoveryOverviewResponse>(
          '/api/recovery/overview',
        )
        if (gen !== recoveryGenerationRef.current) return
        setRecoverySessions(data.overview.killedSessions)
        setRecoveryJobs(data.overview.runningJobs)
        queryClient.setQueryData<RecoveryOverviewCache>(
          TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
          {
            sessions: data.overview.killedSessions,
            jobs: data.overview.runningJobs,
          },
        )
        setRecoveryError('')
      } catch (error) {
        if (gen !== recoveryGenerationRef.current) return
        const message =
          error instanceof Error ? error.message : 'failed to refresh recovery'
        // Recovery can be disabled by config. Treat this as a non-fatal state.
        if (message.toLowerCase().includes('recovery subsystem is disabled')) {
          setRecoverySessions([])
          setRecoveryJobs([])
          queryClient.setQueryData<RecoveryOverviewCache>(
            TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
            {
              sessions: [],
              jobs: [],
            },
          )
          setRecoveryError('')
        } else {
          setRecoveryError(message)
        }
      } finally {
        if (gen === recoveryGenerationRef.current) {
          setRecoveryLoading(false)
        }
      }
    },
    [api, bumpRuntimeMetric, queryClient],
  )

  const resolveTimelineSessionScope = useCallback((scope: string): string => {
    const normalized = scope.trim()
    if (normalized === '' || normalized === 'all') {
      return ''
    }
    if (normalized === 'active') {
      return tabsStateRef.current.activeSession.trim()
    }
    return normalized
  }, [])

  const loadTimeline = useCallback(
    async (options?: { quiet?: boolean }) => {
      const gen = ++timelineGenerationRef.current
      if (!options?.quiet) {
        setTimelineLoading(true)
      }
      const session = resolveTimelineSessionScope(
        timelineSessionFilterRef.current,
      )
      const cacheKey = tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      })
      const cached = queryClient.getQueryData<TmuxTimelineCache>(cacheKey)
      if (cached != null) {
        setTimelineEvents(cached.events)
        setTimelineHasMore(cached.hasMore)
      }
      const queryString = buildTimelineQueryString({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      })
      try {
        const data = await api<TimelineResponse>(
          `/api/tmux/timeline${queryString}`,
        )
        if (gen !== timelineGenerationRef.current) return
        setTimelineEvents(data.events)
        setTimelineHasMore(data.hasMore)
        queryClient.setQueryData<TmuxTimelineCache>(cacheKey, {
          events: data.events,
          hasMore: data.hasMore,
        })
        setTimelineError('')
      } catch (error) {
        if (gen !== timelineGenerationRef.current) return
        const message =
          error instanceof Error ? error.message : 'failed to load timeline'
        setTimelineError(message)
      } finally {
        if (gen === timelineGenerationRef.current) {
          setTimelineLoading(false)
        }
      }
    },
    [
      api,
      queryClient,
      resolveTimelineSessionScope,
      timelineEventType,
      timelineQuery,
      timelineSeverity,
    ],
  )
  useEffect(() => {
    loadTimelineRef.current = (options?: { quiet?: boolean }) => {
      void loadTimeline(options)
    }
  }, [loadTimeline])

  useEffect(() => {
    if (!timelineOpen) {
      return
    }
    const session = resolveTimelineSessionScope(timelineSessionFilter)
    const cached = queryClient.getQueryData<TmuxTimelineCache>(
      tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      }),
    )
    if (cached != null) {
      setTimelineEvents(cached.events)
      setTimelineHasMore(cached.hasMore)
      setTimelineError('')
      setTimelineLoading(false)
    }
    const timeoutID = window.setTimeout(() => {
      void loadTimeline()
    }, 120)
    return () => {
      window.clearTimeout(timeoutID)
    }
  }, [
    loadTimeline,
    queryClient,
    resolveTimelineSessionScope,
    timelineOpen,
    timelineQuery,
    timelineSeverity,
    timelineEventType,
    timelineSessionFilter,
    tabsState.activeSession,
  ])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey)) {
        return
      }
      if (event.key.toLowerCase() !== 'k') {
        return
      }
      event.preventDefault()
      setTimelineOpen(true)
      void loadTimeline({ quiet: true })
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [loadTimeline])

  const loadRecoverySnapshot = useCallback(
    async (snapshotID: number) => {
      setSelectedSnapshotID(snapshotID)
      try {
        const data = await api<RecoverySnapshotResponse>(
          `/api/recovery/snapshots/${snapshotID}`,
        )
        setSelectedSnapshot(data.snapshot)
        setRecoveryError('')
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to load snapshot'
        setRecoveryError(message)
      }
    },
    [api],
  )

  const loadRecoverySnapshots = useCallback(
    async (sessionName: string) => {
      const session = sessionName.trim()
      if (session === '') {
        setRecoverySnapshots([])
        setSelectedSnapshot(null)
        setSelectedSnapshotID(null)
        return
      }
      try {
        const data = await api<RecoverySnapshotsResponse>(
          `/api/recovery/sessions/${encodeURIComponent(session)}/snapshots?limit=25`,
        )
        const snapshots = data.snapshots.map((item) => ({
          id: item.id,
          capturedAt: item.capturedAt,
          windows: item.windows,
          panes: item.panes,
        }))
        setRecoverySnapshots(snapshots)
        if (snapshots.length > 0) {
          const first = snapshots[0]
          setRestoreTargetSession(session)
          await loadRecoverySnapshot(first.id)
        } else {
          setSelectedSnapshot(null)
          setSelectedSnapshotID(null)
        }
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to list snapshots'
        setRecoveryError(message)
      }
    },
    [api, loadRecoverySnapshot],
  )

  const pollRecoveryJob = useCallback(
    (jobID: string) => {
      const startedAt = Date.now()
      const maxDurationMs = 5 * 60 * 1000

      const tick = async () => {
        try {
          const data = await api<RecoveryJobResponse>(
            `/api/recovery/jobs/${encodeURIComponent(jobID)}`,
          )
          setRecoveryJobs((prev) => {
            const next = [data.job, ...prev.filter((j) => j.id !== data.job.id)]
            return next.slice(0, 30)
          })

          if (
            (data.job.status === 'queued' || data.job.status === 'running') &&
            Date.now() - startedAt < maxDurationMs
          ) {
            window.setTimeout(() => {
              void tick()
            }, 1200)
            return
          }

          setRecoveryBusy(false)
          if (data.job.status === 'succeeded') {
            pushSuccessToast(
              'Recovery',
              `session restored to "${data.job.targetSession || data.job.sessionName}"`,
            )
            await refreshSessions()
          } else if (
            data.job.status === 'failed' ||
            data.job.status === 'partial'
          ) {
            pushErrorToast(
              'Recovery',
              data.job.error || 'restore job finished with errors',
            )
          }
          await refreshRecovery({ quiet: true })
        } catch (error) {
          setRecoveryBusy(false)
          const message =
            error instanceof Error
              ? error.message
              : 'failed to track restore progress'
          setRecoveryError(message)
        }
      }

      void tick()
    },
    [api, pushErrorToast, pushSuccessToast, refreshRecovery, refreshSessions],
  )

  const restoreSelectedSnapshot = useCallback(async () => {
    if (selectedSnapshotID === null) return
    setRecoveryBusy(true)
    setRecoveryError('')
    try {
      const data = await api<{ job: RecoveryJob }>(
        `/api/recovery/snapshots/${selectedSnapshotID}/restore`,
        {
          method: 'POST',
          body: JSON.stringify({
            mode: restoreMode,
            conflictPolicy: restoreConflictPolicy,
            targetSession: restoreTargetSession.trim(),
          }),
        },
      )
      setRecoveryJobs((prev) => [
        data.job,
        ...prev.filter((item) => item.id !== data.job.id),
      ])
      pollRecoveryJob(data.job.id)
    } catch (error) {
      setRecoveryBusy(false)
      const message =
        error instanceof Error ? error.message : 'failed to start restore'
      setRecoveryError(message)
      pushErrorToast('Recovery', message)
    }
  }, [
    api,
    pollRecoveryJob,
    pushErrorToast,
    restoreConflictPolicy,
    restoreMode,
    restoreTargetSession,
    selectedSnapshotID,
  ])

  const archiveRecoverySession = useCallback(
    async (sessionName: string) => {
      try {
        await api<void>(
          `/api/recovery/sessions/${encodeURIComponent(sessionName)}/archive`,
          {
            method: 'POST',
            body: '{}',
          },
        )
        await refreshRecovery({ quiet: true })
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to archive session'
        setRecoveryError(message)
      }
    },
    [api, refreshRecovery],
  )

  const createSession = useCallback(
    async (name: string, cwd: string) => {
      const sessionName = name.trim()
      if (!sessionName) {
        setConnection('error', 'session name required')
        pushErrorToast('Create Session', 'session name required')
        return
      }

      pendingKillSessionsRef.current.delete(sessionName)
      clearPendingInspectorSessionState(sessionName)
      clearPendingSessionRenamesForName(sessionName)

      const previousActiveSession = tabsStateRef.current.activeSession
      const sessionAlreadyExists = sessionsRef.current.some(
        (item) => item.name === sessionName,
      )
      if (!sessionAlreadyExists) {
        const optimisticAt = new Date().toISOString()
        pendingCreateSessionsRef.current.set(sessionName, optimisticAt)
        setSessions((prev) =>
          upsertOptimisticAttachedSession(prev, sessionName, optimisticAt),
        )
        dispatchTabs({ type: 'activate', session: sessionName })
        setConnection('connecting', `creating ${sessionName}`)
        setInspectorError('')
      }

      try {
        await api<{ name: string }>('/api/tmux/sessions', {
          method: 'POST',
          body: JSON.stringify({ name: sessionName, cwd }),
        })

        activateSession(sessionName)
        setConnection('connecting', `opening ${sessionName}`)
        void refreshInspector(sessionName)
        void refreshSessions()
        pushSuccessToast('Create Session', `session "${sessionName}" created`)
      } catch (error) {
        pendingCreateSessionsRef.current.delete(sessionName)
        const msg =
          error instanceof Error ? error.message : 'failed to create session'
        if (!sessionAlreadyExists) {
          setSessions((prev) =>
            prev.filter((item) => item.name !== sessionName),
          )
          dispatchTabs({ type: 'remove', session: sessionName })
          const currentActiveSession = tabsStateRef.current.activeSession
          if (
            currentActiveSession === sessionName &&
            previousActiveSession !== '' &&
            previousActiveSession !== sessionName
          ) {
            dispatchTabs({ type: 'activate', session: previousActiveSession })
          }
        }
        setConnection('error', msg)
        pushErrorToast('Create Session', msg)
      }
    },
    [
      activateSession,
      api,
      clearPendingInspectorSessionState,
      clearPendingSessionRenamesForName,
      pushErrorToast,
      pushSuccessToast,
      refreshInspector,
      refreshSessions,
      setConnection,
    ],
  )

  const killSession = useCallback(
    async (name: string) => {
      const sessionName = name.trim()
      if (sessionName === '') {
        return
      }

      const activeBeforeKill = tabsStateRef.current.activeSession
      const wasActive = activeBeforeKill === sessionName
      const hadSession = sessionsRef.current.some(
        (item) => item.name === sessionName,
      )

      pendingKillSessionsRef.current.add(sessionName)
      if (hadSession) {
        setSessions((prev) => prev.filter((item) => item.name !== sessionName))
      }
      pendingCreateSessionsRef.current.delete(sessionName)
      clearPendingInspectorSessionState(sessionName)
      clearPendingSessionRenamesForName(sessionName)
      dispatchTabs({ type: 'remove', session: sessionName })
      if (wasActive) {
        closeCurrentSocket('session killed')
        resetTerminal()
        setConnection('disconnected', 'session killed')
      }

      try {
        const killURL = `/api/tmux/sessions/${encodeURIComponent(sessionName)}?confirm=1`
        await api<void>(killURL, {
          method: 'DELETE',
          headers: {
            'X-Sentinel-Guardrail-Confirm': 'true',
          },
        })

        void refreshSessions()
        pushSuccessToast('Kill Session', `session "${sessionName}" killed`)
      } catch (error) {
        pendingKillSessionsRef.current.delete(sessionName)
        if (hadSession) {
          void refreshSessions()
        }
        if (activeBeforeKill !== '') {
          dispatchTabs({ type: 'activate', session: activeBeforeKill })
        }
        const msg =
          error instanceof Error ? error.message : 'failed to kill session'
        setConnection('error', msg)
        pushErrorToast('Kill Session', msg)
      }
    },
    [
      api,
      clearPendingInspectorSessionState,
      clearPendingSessionRenamesForName,
      closeCurrentSocket,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      resetTerminal,
      setConnection,
    ],
  )

  const handleConfirmKill = useCallback(() => {
    if (killDialogSession) {
      void killSession(killDialogSession)
    }
    setKillDialogSession(null)
  }, [killDialogSession, killSession])

  const renameActive = useCallback(
    async (targetSession: string, newName: string) => {
      const active = targetSession.trim()
      if (!active) {
        setConnection('error', 'no active session')
        return
      }
      const sanitized = slugifyTmuxName(newName).trim()
      if (!sanitized || sanitized === active) return
      if (
        sessionsRef.current.some(
          (item) => item.name === sanitized && item.name !== active,
        )
      ) {
        const msg = `session "${sanitized}" already exists`
        setConnection('error', msg)
        pushErrorToast('Rename Session', msg)
        return
      }
      const changedAt = new Date().toISOString()
      clearPendingSessionRenamesForName(active)
      clearPendingSessionRenamesForName(sanitized)
      pendingRenameSessionsRef.current.set(active, sanitized)
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? { ...item, name: sanitized, activityAt: changedAt }
            : item,
        ),
      )
      dispatchTabs({ type: 'rename', oldName: active, newName: sanitized })
      try {
        await api<{ name: string }>(
          `/api/tmux/sessions/${encodeURIComponent(active)}`,
          { method: 'PATCH', body: JSON.stringify({ newName: sanitized }) },
        )
        void refreshSessions()
        setConnection(
          connectionState === 'connected' ? 'connected' : 'disconnected',
          'session renamed',
        )
        pushSuccessToast('Rename Session', `"${active}" -> "${sanitized}"`)
      } catch (error) {
        pendingRenameSessionsRef.current.delete(active)
        setSessions((prev) =>
          prev.map((item) =>
            item.name === sanitized ? { ...item, name: active } : item,
          ),
        )
        dispatchTabs({ type: 'rename', oldName: sanitized, newName: active })
        void refreshSessions()
        const msg =
          error instanceof Error ? error.message : 'failed to rename session'
        setConnection('error', msg)
        pushErrorToast('Rename Session', msg)
      }
    },
    [
      api,
      clearPendingSessionRenamesForName,
      connectionState,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      setConnection,
    ],
  )

  const setSessionIcon = useCallback(
    async (session: string, icon: string) => {
      const target = session.trim()
      if (target === '') return
      const previousIcon =
        sessionsRef.current.find((item) => item.name === target)?.icon ?? ''
      setSessions((prev) =>
        prev.map((item) =>
          item.name === target
            ? { ...item, icon, activityAt: new Date().toISOString() }
            : item,
        ),
      )
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(target)}/icon`,
          {
            method: 'PATCH',
            body: JSON.stringify({ icon }),
          },
        )
      } catch {
        setSessions((prev) =>
          prev.map((item) =>
            item.name === target ? { ...item, icon: previousIcon } : item,
          ),
        )
        pushErrorToast('Change Icon', 'failed to change session icon')
      }
    },
    [api, pushErrorToast],
  )

  const handleOpenRenameDialogForSession = useCallback(
    (session: string) => {
      const target = session.trim()
      if (!target) {
        setConnection('error', 'no active session')
        return
      }
      setRenameSessionTarget(target)
      setRenameValue(target)
      setRenameDialogOpen(true)
    },
    [setConnection],
  )

  const handleSubmitRename = useCallback(() => {
    const target = renameSessionTarget?.trim() ?? ''
    if (!target) return
    setRenameDialogOpen(false)
    setRenameSessionTarget(null)
    void renameActive(target, renameValue)
  }, [renameActive, renameSessionTarget, renameValue])

  const renameWindow = useCallback(
    async (index: number, newName: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active) {
        setConnection('error', 'no active session')
        pushErrorToast('Rename Window', 'no active session')
        return
      }
      const sanitized = slugifyTmuxName(newName).trim()
      if (!sanitized) {
        pushErrorToast('Rename Window', 'window name required')
        return
      }
      setWindows((prev) =>
        prev.map((w) => (w.index === index ? { ...w, name: sanitized } : w)),
      )
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-window`,
          {
            method: 'POST',
            body: JSON.stringify({ index, name: sanitized }),
          },
        )
        pushSuccessToast('Rename Window', `window #${index} -> "${sanitized}"`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename window'
        setInspectorError(msg)
        pushErrorToast('Rename Window', msg)
        void refreshInspector(active, { background: true })
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshInspector, setConnection],
  )

  const handleOpenRenameWindow = useCallback((windowInfo: WindowInfo) => {
    setRenameWindowIndex(windowInfo.index)
    setRenameWindowValue(slugifyTmuxName(windowInfo.name))
    setRenameWindowDialogOpen(true)
  }, [])

  const handleSubmitRenameWindow = useCallback(() => {
    const index = renameWindowIndex
    if (index === null) return
    setRenameWindowDialogOpen(false)
    setRenameWindowIndex(null)
    void renameWindow(index, renameWindowValue)
  }, [renameWindow, renameWindowIndex, renameWindowValue])

  const renamePane = useCallback(
    async (paneID: string, title: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active) {
        setConnection('error', 'no active session')
        pushErrorToast('Rename Pane', 'no active session')
        return
      }
      if (isPendingSplitPaneID(paneID)) {
        pushErrorToast('Rename Pane', 'wait for pane creation to finish')
        return
      }
      const sanitized = slugifyTmuxName(title).trim()
      if (!sanitized) {
        pushErrorToast('Rename Pane', 'pane title required')
        return
      }
      setPanes((prev) =>
        prev.map((p) => (p.paneId === paneID ? { ...p, title: sanitized } : p)),
      )
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-pane`,
          {
            method: 'POST',
            body: JSON.stringify({ paneId: paneID, title: sanitized }),
          },
        )
        pushSuccessToast('Rename Pane', `pane ${paneID} renamed`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename pane'
        setInspectorError(msg)
        pushErrorToast('Rename Pane', msg)
        void refreshInspector(active, { background: true })
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshInspector, setConnection],
  )

  const handleOpenRenamePane = useCallback((paneInfo: PaneInfo) => {
    const initialTitle =
      paneInfo.title.trim() !== '' ? paneInfo.title : paneInfo.paneId
    setRenamePaneID(paneInfo.paneId)
    setRenamePaneValue(slugifyTmuxName(initialTitle))
    setRenamePaneDialogOpen(true)
  }, [])

  const handleSubmitRenamePane = useCallback(() => {
    const paneID = renamePaneID
    if (paneID === null) return
    setRenamePaneDialogOpen(false)
    setRenamePaneID(null)
    void renamePane(paneID, renamePaneValue)
  }, [renamePane, renamePaneID, renamePaneValue])

  const selectWindow = useCallback(
    (windowIndex: number) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      if (activeWindowIndexRef.current === windowIndex) return
      setInspectorError('')
      setActiveWindowIndexOverride(windowIndex)
      const preferredPaneID =
        panes.find((p) => p.windowIndex === windowIndex && p.active)?.paneId ??
        panes.find((p) => p.windowIndex === windowIndex)?.paneId ??
        null
      setActivePaneIDOverride(preferredPaneID)
      setWindows((prev) =>
        prev.map((w) => ({ ...w, active: w.index === windowIndex })),
      )
      if (preferredPaneID !== null)
        setPanes((prev) =>
          prev.map((p) => ({ ...p, active: p.paneId === preferredPaneID })),
        )
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/select-window`,
        { method: 'POST', body: JSON.stringify({ index: windowIndex }) },
      ).catch((error) => {
        const msg =
          error instanceof Error ? error.message : 'failed to switch window'
        setInspectorError(msg)
        pushErrorToast('Switch Window', msg)
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
        void refreshInspector(active, { background: true })
      })
    },
    [api, panes, pushErrorToast, refreshInspector],
  )

  const selectPane = useCallback(
    (paneID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active || !paneID.trim()) return
      if (isPendingSplitPaneID(paneID)) return
      if (activePaneIDRef.current === paneID) return
      const paneInfo = panes.find((p) => p.paneId === paneID)
      setInspectorError('')
      setActivePaneIDOverride(paneID)
      if (paneInfo) setActiveWindowIndexOverride(paneInfo.windowIndex)
      setPanes((prev) =>
        prev.map((p) => ({ ...p, active: p.paneId === paneID })),
      )
      if (paneInfo)
        setWindows((prev) =>
          prev.map((w) => ({ ...w, active: w.index === paneInfo.windowIndex })),
        )
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/select-pane`,
        { method: 'POST', body: JSON.stringify({ paneId: paneID }) },
      ).catch((error) => {
        const msg =
          error instanceof Error ? error.message : 'failed to switch pane'
        setInspectorError(msg)
        pushErrorToast('Switch Pane', msg)
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
        void refreshInspector(active, { background: true })
      })
    },
    [api, panes, pushErrorToast, refreshInspector],
  )

  const createWindow = useCallback(() => {
    const active = tabsStateRef.current.activeSession
    if (!active) return
    const changedAt = new Date().toISOString()
    const nextIdx = windows.reduce((h, w) => Math.max(h, w.index), -1) + 1
    removePendingWindowClose(pendingCloseWindowsRef.current, active, nextIdx)
    addPendingWindowCreate(pendingCreateWindowsRef.current, active, nextIdx)
    clearPendingWindowPaneFloor(
      pendingWindowPaneFloorsRef.current,
      active,
      nextIdx,
    )
    setInspectorError('')
    setSessions((prev) =>
      prev.map((item) =>
        item.name === active
          ? {
              ...item,
              windows: item.windows + 1,
              panes: item.panes + 1,
              activityAt: changedAt,
            }
          : item,
      ),
    )
    setWindows((prev) => [
      ...prev
        .filter((w) => w.index !== nextIdx)
        .map((w) => ({ ...w, active: false })),
      { session: active, index: nextIdx, name: 'new', active: true, panes: 1 },
    ])
    setPanes((prev) => prev.map((p) => ({ ...p, active: false })))
    setActiveWindowIndexOverride(nextIdx)
    setActivePaneIDOverride(null)
    void api<void>(
      `/api/tmux/sessions/${encodeURIComponent(active)}/new-window`,
      { method: 'POST', body: '{}' },
    )
      .then(() => {
        if (!eventsSocketConnectedRef.current) {
          void refreshInspector(active, { background: true })
          void refreshSessions()
        }
      })
      .catch((error) => {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          active,
          nextIdx,
        )
        const msg =
          error instanceof Error ? error.message : 'failed to create window'
        setInspectorError(msg)
        pushErrorToast('New Window', msg)
        void refreshInspector(active, { background: true })
        void refreshSessions()
      })
  }, [api, pushErrorToast, refreshInspector, refreshSessions, windows])

  const closeWindow = useCallback(
    (windowIndex: number) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      const removedPaneCount = panes.filter(
        (paneInfo) => paneInfo.windowIndex === windowIndex,
      ).length
      const changedAt = new Date().toISOString()
      removePendingWindowCreate(
        pendingCreateWindowsRef.current,
        active,
        windowIndex,
      )
      addPendingWindowClose(pendingCloseWindowsRef.current, active, windowIndex)
      clearPendingWindowPaneFloor(
        pendingWindowPaneFloorsRef.current,
        active,
        windowIndex,
      )
      const rem = windows.filter((w) => w.index !== windowIndex)
      const remP = panes.filter((p) => p.windowIndex !== windowIndex)
      const ord = [...rem].sort((a, b) => a.index - b.index)
      let nextWI: number | null = null
      if (
        activeWindowIndex !== null &&
        ord.some((w) => w.index === activeWindowIndex)
      )
        nextWI = activeWindowIndex
      if (nextWI === null) {
        const h = ord.find((w) => w.index > windowIndex)
        nextWI = h ? h.index : (ord.at(-1)?.index ?? null)
      }
      let nextPI: string | null = null
      if (nextWI !== null) {
        const ap = remP.find((p) => p.windowIndex === nextWI && p.active)
        nextPI = ap
          ? ap.paneId
          : (remP.find((p) => p.windowIndex === nextWI)?.paneId ?? null)
      }
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                windows: Math.max(0, item.windows - 1),
                panes: Math.max(0, item.panes - Math.max(1, removedPaneCount)),
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows(rem.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-window`,
        { method: 'POST', body: JSON.stringify({ index: windowIndex }) },
      )
        .then(() => {
          if (!eventsSocketConnectedRef.current) {
            void refreshInspector(active, { background: true })
            void refreshSessions()
          }
        })
        .catch((error) => {
          removePendingWindowClose(
            pendingCloseWindowsRef.current,
            active,
            windowIndex,
          )
          const msg =
            error instanceof Error ? error.message : 'failed to close window'
          setInspectorError(msg)
          pushErrorToast('Kill Window', msg)
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
    },
    [
      activeWindowIndex,
      api,
      panes,
      pushErrorToast,
      refreshInspector,
      refreshSessions,
      windows,
    ],
  )

  const closePane = useCallback(
    (paneID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active || !paneID.trim()) return
      if (isPendingSplitPaneID(paneID)) return
      const changedAt = new Date().toISOString()
      addPendingPaneClose(pendingClosePanesRef.current, active, paneID)
      const removed = panes.find((p) => p.paneId === paneID)
      const remP = panes.filter((p) => p.paneId !== paneID)
      const countByW = new Map<number, number>()
      for (const p of remP)
        countByW.set(p.windowIndex, (countByW.get(p.windowIndex) ?? 0) + 1)
      const remW = windows
        .filter((w) => countByW.has(w.index))
        .map((w) => ({ ...w, panes: countByW.get(w.index) ?? 0 }))
      const ord = [...remW].sort((a, b) => a.index - b.index)
      let nextWI: number | null = null
      if (
        activeWindowIndex !== null &&
        ord.some((w) => w.index === activeWindowIndex)
      )
        nextWI = activeWindowIndex
      if (
        nextWI === null &&
        removed &&
        ord.some((w) => w.index === removed.windowIndex)
      )
        nextWI = removed.windowIndex
      if (nextWI === null) nextWI = ord.at(0)?.index ?? null
      let nextPI: string | null = null
      if (activePaneID !== null && remP.some((p) => p.paneId === activePaneID))
        nextPI = activePaneID
      if (nextPI === null && nextWI !== null) {
        const ap = remP.find((p) => p.windowIndex === nextWI && p.active)
        nextPI = ap
          ? ap.paneId
          : (remP.find((p) => p.windowIndex === nextWI)?.paneId ?? null)
      }
      const removedWindow = removed
        ? !remW.some((w) => w.index === removed.windowIndex)
        : false
      if (removed && removedWindow) {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          active,
          removed.windowIndex,
        )
        addPendingWindowClose(
          pendingCloseWindowsRef.current,
          active,
          removed.windowIndex,
        )
      }
      if (removed) {
        clearPendingWindowPaneFloor(
          pendingWindowPaneFloorsRef.current,
          active,
          removed.windowIndex,
        )
      }
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                panes: Math.max(0, item.panes - 1),
                windows: removedWindow
                  ? Math.max(0, item.windows - 1)
                  : item.windows,
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows(remW.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-pane`,
        { method: 'POST', body: JSON.stringify({ paneId: paneID }) },
      )
        .then(() => {
          if (!eventsSocketConnectedRef.current) {
            void refreshInspector(active, { background: true })
            void refreshSessions()
          }
        })
        .catch((error) => {
          removePendingPaneClose(pendingClosePanesRef.current, active, paneID)
          if (removedWindow && removed) {
            removePendingWindowClose(
              pendingCloseWindowsRef.current,
              active,
              removed.windowIndex,
            )
          }
          const msg =
            error instanceof Error ? error.message : 'failed to close pane'
          setInspectorError(msg)
          pushErrorToast('Kill Pane', msg)
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
    },
    [
      activePaneID,
      activeWindowIndex,
      api,
      panes,
      pushErrorToast,
      refreshInspector,
      refreshSessions,
      windows,
    ],
  )

  const splitPane = useCallback(
    (direction: 'vertical' | 'horizontal') => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      const changedAt = new Date().toISOString()
      const inWin =
        activeWindowIndex === null
          ? []
          : panes.filter((p) => p.windowIndex === activeWindowIndex)
      const targetID =
        inWin.find((p) => p.paneId === activePaneID)?.paneId ??
        inWin.find((p) => p.active)?.paneId ??
        inWin[0]?.paneId
      if (!targetID) {
        pushErrorToast('Split Pane', 'no pane available to split')
        return
      }
      const target = panes.find((p) => p.paneId === targetID)
      if (!target) {
        pushErrorToast('Split Pane', 'target pane is not available')
        return
      }
      const expectedPaneFloor =
        (windows.find((windowInfo) => windowInfo.index === target.windowIndex)
          ?.panes ?? inWin.length) + 1
      const pendingPaneID = buildPendingSplitPaneID(
        active,
        target.windowIndex,
        inWin.length,
      )
      setPendingWindowPaneFloor(
        pendingWindowPaneFloorsRef.current,
        active,
        target.windowIndex,
        expectedPaneFloor,
      )
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                panes: item.panes + 1,
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows((prev) =>
        prev.map((w) => ({
          ...w,
          active: w.index === target.windowIndex,
          panes: w.index === target.windowIndex ? w.panes + 1 : w.panes,
        })),
      )
      setPanes((prev) => {
        const inWindow = prev.filter(
          (paneInfo) => paneInfo.windowIndex === target.windowIndex,
        )
        const nextPaneIndex =
          inWindow.reduce(
            (highest, paneInfo) => Math.max(highest, paneInfo.paneIndex),
            -1,
          ) + 1
        const withoutPending = prev.filter(
          (paneInfo) => paneInfo.paneId !== pendingPaneID,
        )
        return [
          ...withoutPending.map((p) => ({ ...p, active: false })),
          {
            session: active,
            windowIndex: target.windowIndex,
            paneIndex: nextPaneIndex,
            paneId: pendingPaneID,
            title: 'new',
            active: true,
            tty: '',
            hasUnread: false,
          },
        ]
      })
      setActiveWindowIndexOverride(target.windowIndex)
      setActivePaneIDOverride(pendingPaneID)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/split-pane`,
        {
          method: 'POST',
          body: JSON.stringify({ paneId: targetID, direction }),
        },
      )
        .then(() => {
          if (!eventsSocketConnectedRef.current) {
            void refreshInspector(active, { background: true })
          }
        })
        .catch((error) => {
          clearPendingWindowPaneFloor(
            pendingWindowPaneFloorsRef.current,
            active,
            target.windowIndex,
          )
          const msg =
            error instanceof Error ? error.message : 'failed to split pane'
          setInspectorError(msg)
          pushErrorToast('Split Pane', msg)
          void refreshInspector(active, { background: true })
        })
    },
    [
      activePaneID,
      activeWindowIndex,
      api,
      panes,
      pushErrorToast,
      refreshInspector,
    ],
  )

  const reorderTabs = useCallback((from: number, to: number) => {
    dispatchTabs({ type: 'reorder', from, to })
  }, [])

  const closeTab = useCallback(
    (name: string) => {
      const wasActive = tabsStateRef.current.activeSession === name
      const nextCount = tabsStateRef.current.openTabs.filter(
        (t) => t !== name,
      ).length
      setSessions((prev) =>
        prev.map((item) =>
          item.name === name
            ? { ...item, attached: Math.max(0, item.attached - 1) }
            : item,
        ),
      )
      if (wasActive) {
        closeCurrentSocket('detached')
        resetTerminal()
      }
      dispatchTabs({ type: 'close', session: name })
      if (wasActive && nextCount === 0)
        setConnection('disconnected', 'detached')
    },
    [closeCurrentSocket, resetTerminal, setConnection],
  )

  const detachSession = useCallback(
    (name: string) => {
      const isOpen = tabsStateRef.current.openTabs.includes(name)
      if (!isOpen) return
      closeTab(name)
    },
    [closeTab],
  )

  const syncActivityDelta = useCallback(
    async (options?: { reason?: string; force?: boolean }) => {
      if (deltaSyncInFlightRef.current) {
        return
      }
      if (!options?.force && !eventsSocketConnectedRef.current) {
        return
      }
      const now = Date.now()
      if (now - lastDeltaSyncAtRef.current < 900) {
        return
      }

      deltaSyncInFlightRef.current = true
      lastDeltaSyncAtRef.current = now
      bumpRuntimeMetric('deltaSyncCount')
      try {
        const since = Math.max(0, Math.trunc(lastGlobalRevRef.current))
        const data = await api<ActivityDeltaResponse>(
          `/api/tmux/activity/delta?since=${since}&limit=300`,
        )

        const responseGlobalRev =
          typeof data.globalRev === 'number' &&
          Number.isFinite(data.globalRev) &&
          data.globalRev >= 0
            ? Math.trunc(data.globalRev)
            : 0
        if (responseGlobalRev > lastGlobalRevRef.current) {
          lastGlobalRevRef.current = responseGlobalRev
        }

        applySessionActivityPatches(data.sessionPatches)
        applyInspectorProjectionPatches(data.inspectorPatches)

        if (data.overflow === true) {
          bumpRuntimeMetric('deltaOverflowCount')
          void refreshSessions()
          const active = tabsStateRef.current.activeSession.trim()
          if (active !== '') {
            void refreshInspector(active, { background: true })
          }
        }
      } catch {
        bumpRuntimeMetric('deltaSyncErrors')
      } finally {
        deltaSyncInFlightRef.current = false
      }
    },
    [
      api,
      applyInspectorProjectionPatches,
      applySessionActivityPatches,
      bumpRuntimeMetric,
      refreshInspector,
      refreshSessions,
    ],
  )

  const refreshAllState = useCallback(
    (options?: { quietRecovery?: boolean }) => {
      void refreshSessions()
      const active = tabsStateRef.current.activeSession.trim()
      if (active !== '') {
        // Use foreground mode so inspectorLoading is properly managed.
        // A background call can race with the inspector-sync effect and
        // leave inspectorLoading stuck at true when both fire on mount.
        void refreshInspector(active)
      }
      void refreshRecovery({ quiet: options?.quietRecovery ?? true })
    },
    [refreshInspector, refreshRecovery, refreshSessions],
  )

  useEffect(() => {
    // Initial sync on page load.
    refreshAllState({ quietRecovery: false })
  }, [refreshAllState])

  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        if (eventsSocketConnected) {
          void syncActivityDelta({ reason: 'visibility-visible' })
          return
        }
        refreshAllState({ quietRecovery: true })
      }
    }
    const onOnline = () => {
      if (eventsSocketConnected) {
        void syncActivityDelta({ reason: 'browser-online' })
        return
      }
      refreshAllState({ quietRecovery: true })
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', onOnline)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', onOnline)
    }
  }, [eventsSocketConnected, refreshAllState, syncActivityDelta])

  useEffect(() => {
    // Adaptive fallback: poll only while WS events channel is disconnected.
    if (eventsSocketConnected) return
    bumpRuntimeMetric('fallbackRefreshCount')
    refreshAllState({ quietRecovery: true })
    const id = window.setInterval(() => {
      bumpRuntimeMetric('fallbackRefreshCount')
      refreshAllState({ quietRecovery: true })
    }, 8_000)
    return () => {
      window.clearInterval(id)
    }
  }, [bumpRuntimeMetric, eventsSocketConnected, refreshAllState])

  useEffect(() => {
    if (recoverySessions.length === 0) {
      setSelectedRecoverySession(null)
      setRecoverySnapshots([])
      setSelectedSnapshot(null)
      setSelectedSnapshotID(null)
      return
    }
    if (
      selectedRecoverySession === null ||
      !recoverySessions.some((item) => item.name === selectedRecoverySession)
    ) {
      setSelectedRecoverySession(recoverySessions[0].name)
    }
  }, [recoverySessions, selectedRecoverySession])

  useEffect(() => {
    if (!recoveryDialogOpen || selectedRecoverySession === null) return
    void loadRecoverySnapshots(selectedRecoverySession)
  }, [loadRecoverySnapshots, recoveryDialogOpen, selectedRecoverySession])

  useEffect(() => {
    let reconnectTimer: number | null = null
    let closed = false
    let socket: WebSocket | null = null

    const parseEventID = (value: unknown): number => {
      if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
        return 0
      }
      return Math.trunc(value)
    }

    const parseGlobalRev = (value: unknown): number => {
      if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
        return 0
      }
      return Math.trunc(value)
    }

    const schedule = (
      kind: 'sessions' | 'inspector' | 'recovery' | 'timeline',
      options?: { minGapMs?: number },
    ) => {
      if (refreshTimerRef.current[kind] !== null) return
      let delay = 180
      if (kind === 'sessions' && (options?.minGapMs ?? 0) > 0) {
        const elapsed = Date.now() - lastSessionsRefreshAtRef.current
        const gap = options?.minGapMs ?? 0
        if (elapsed < gap) {
          delay = Math.max(delay, gap - elapsed)
        }
      }
      refreshTimerRef.current[kind] = window.setTimeout(() => {
        refreshTimerRef.current[kind] = null
        if (kind === 'sessions') {
          void refreshSessions()
          return
        }
        if (kind === 'inspector') {
          const active = tabsStateRef.current.activeSession
          if (active.trim() !== '') {
            void refreshInspector(active, { background: true })
          }
          return
        }
        if (kind === 'timeline') {
          loadTimelineRef.current({ quiet: true })
          return
        }
        void refreshRecovery({ quiet: true })
      }, delay)
    }

    const connect = () => {
      const wsURL = new URL('/ws/events', window.location.origin)
      socket = new WebSocket(
        wsURL.toString().replace(/^http/, 'ws'),
        buildWSProtocols(token),
      )

      socket.onopen = () => {
        bumpRuntimeMetric('wsOpenCount')
        wsReconnectAttemptsRef.current = 0
        presenceSocketRef.current = socket
        setEventsSocketConnected(true)
        sendPresenceOverWS(true)
        // Reconcile missed revisions without forcing full inspector reload.
        void syncActivityDelta({ reason: 'events-open', force: true })
      }

      socket.onmessage = (event) => {
        bumpRuntimeMetric('wsMessages')
        if (typeof event.data !== 'string') return
        try {
          const msg = JSON.parse(event.data) as SeenAckMessage & {
            globalRev?: number
            payload?: {
              session?: string
              action?: string
              globalRev?: number
              sessionPatches?: Array<SessionActivityPatch>
              inspectorPatches?: Array<InspectorSessionPatch>
              sessions?: Array<string>
              decision?: {
                message?: string
              }
            }
          }

          const messageEventID = parseEventID(msg.eventId)
          const previousEventID = lastEventIDRef.current
          if (messageEventID > lastEventIDRef.current) {
            lastEventIDRef.current = messageEventID
          }
          const hasEventGap =
            previousEventID > 0 && messageEventID > previousEventID + 1

          const messageGlobalRev = Math.max(
            parseGlobalRev(msg.globalRev),
            parseGlobalRev(msg.payload?.globalRev),
          )
          if (messageGlobalRev > lastGlobalRevRef.current) {
            lastGlobalRevRef.current = messageGlobalRev
          }

          if (msg.type === 'tmux.seen.ack') {
            applySessionActivityPatches(msg.sessionPatches)
            applyInspectorProjectionPatches(msg.inspectorPatches)
            const requestId = (msg.requestId ?? '').trim()
            if (requestId !== '') {
              const settle = seenAckWaitersRef.current.get(requestId)
              if (settle) {
                seenAckWaitersRef.current.delete(requestId)
                settle(true)
              }
            }
            return
          }
          switch (msg.type) {
            case 'tmux.activity.updated': {
              applyInspectorProjectionPatches(msg.payload?.inspectorPatches)
              const decision = shouldRefreshSessionsFromEvent(
                'activity',
                applySessionActivityPatches(msg.payload?.sessionPatches),
              )
              if (decision.refresh) {
                if (typeof decision.minGapMs === 'number') {
                  schedule('sessions', { minGapMs: decision.minGapMs })
                } else {
                  schedule('sessions')
                }
              }
              if (hasEventGap) {
                void syncActivityDelta({
                  reason: 'activity-event-gap',
                  force: true,
                })
              }
              break
            }
            case 'tmux.sessions.updated': {
              applyInspectorProjectionPatches(msg.payload?.inspectorPatches)
              const decision = shouldRefreshSessionsFromEvent(
                msg.payload?.action,
                applySessionActivityPatches(msg.payload?.sessionPatches),
              )
              if (decision.refresh) {
                if (typeof decision.minGapMs === 'number') {
                  schedule('sessions', { minGapMs: decision.minGapMs })
                } else {
                  schedule('sessions')
                }
              }
              if (hasEventGap) {
                void syncActivityDelta({
                  reason: 'sessions-event-gap',
                  force: true,
                })
              }
              break
            }
            case 'tmux.inspector.updated': {
              const action = (msg.payload?.action ?? '').trim().toLowerCase()
              if (action === '') {
                break
              }
              if (action === 'seen') {
                break
              }
              const target = msg.payload?.session?.trim() ?? ''
              const active = tabsStateRef.current.activeSession
              if (target !== '' && target === active) {
                schedule('inspector')
              }
              break
            }
            case 'tmux.timeline.updated': {
              if (!timelineOpenRef.current) {
                break
              }
              let trackedSession = ''
              const rawScope = timelineSessionFilterRef.current.trim()
              if (rawScope === '' || rawScope === 'all') {
                trackedSession = 'all'
              } else if (rawScope === 'active') {
                const active = tabsStateRef.current.activeSession.trim()
                trackedSession = active === '' ? 'all' : active
              } else {
                trackedSession = rawScope
              }
              if (
                shouldRefreshTimelineFromEvent(
                  msg.payload?.sessions,
                  trackedSession,
                )
              ) {
                schedule('timeline')
              }
              break
            }
            case 'tmux.guardrail.blocked': {
              const message =
                msg.payload?.decision?.message ??
                'Operation blocked by guardrail policy'
              pushErrorToast('Guardrail', message)
              break
            }
            case 'tmux.auth.expired': {
              settlePendingSeenAcks(false)
              if (tokenRequired) {
                setToken('')
              }
              socket?.close()
              break
            }
            case 'recovery.overview.updated':
            case 'recovery.job.updated':
              schedule('recovery')
              break
            default:
              break
          }
        } catch {
          // Ignore non-JSON control messages.
        }
      }

      socket.onerror = () => {
        socket?.close()
      }

      socket.onclose = () => {
        bumpRuntimeMetric('wsCloseCount')
        settlePendingSeenAcks(false)
        presenceSocketRef.current = null
        setEventsSocketConnected(false)
        if (closed) return
        wsReconnectAttemptsRef.current += 1
        bumpRuntimeMetric('wsReconnects')
        const attempt = wsReconnectAttemptsRef.current
        const expo = Math.min(8, attempt)
        const baseDelay = Math.min(10_000, 500 * 2 ** expo)
        const jitter = Math.floor(Math.random() * 300)
        reconnectTimer = window.setTimeout(() => {
          connect()
        }, baseDelay + jitter)
      }
    }

    connect()

    return () => {
      closed = true
      settlePendingSeenAcks(false)
      presenceSocketRef.current = null
      setEventsSocketConnected(false)
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
      }
      for (const key of [
        'sessions',
        'inspector',
        'recovery',
        'timeline',
      ] as const) {
        const id = refreshTimerRef.current[key]
        if (id !== null) {
          window.clearTimeout(id)
          refreshTimerRef.current[key] = null
        }
      }
      if (socket !== null) {
        socket.close()
      }
    }
  }, [
    bumpRuntimeMetric,
    applyInspectorProjectionPatches,
    applySessionActivityPatches,
    refreshInspector,
    refreshRecovery,
    refreshSessions,
    pushErrorToast,
    sendPresenceOverWS,
    settlePendingSeenAcks,
    syncActivityDelta,
    tokenRequired,
    setToken,
    token,
  ])

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
          defaultCwd={defaultCwd}
          filter={filter}
          token={token}
          tmuxUnavailable={tmuxUnavailable}
          recoveryKilledCount={
            recoverySessions.filter((item) => item.state === 'killed').length
          }
          onFilterChange={setFilter}
          onTokenChange={setToken}
          onCreate={(name, cwd) => {
            void createSession(name, cwd)
          }}
          onOpenRecovery={() => {
            setRecoveryDialogOpen(true)
          }}
          onAttach={activateSession}
          onRename={handleOpenRenameDialogForSession}
          onDetach={detachSession}
          onKill={setKillDialogSession}
          onChangeIcon={setSessionIcon}
        />
      }
    >
      <TmuxTerminalPanel
        connectionState={connectionState}
        statusDetail={statusDetail}
        openTabs={tabsState.openTabs}
        activeSession={tabsState.activeSession}
        inspectorLoading={inspectorLoading}
        inspectorError={inspectorError}
        windows={windows}
        panes={panes}
        activeWindowIndex={activeWindowIndex}
        activePaneID={activePaneID}
        termCols={termCols}
        termRows={termRows}
        getTerminalHostRef={getTerminalHostRef}
        onToggleSidebarOpen={() => layout.setSidebarOpen((prev) => !prev)}
        onSelectWindow={selectWindow}
        onSelectPane={selectPane}
        onRenameWindow={handleOpenRenameWindow}
        onRenamePane={handleOpenRenamePane}
        onCreateWindow={createWindow}
        onCloseWindow={closeWindow}
        onSplitPaneVertical={() => splitPane('vertical')}
        onSplitPaneHorizontal={() => splitPane('horizontal')}
        onClosePane={closePane}
        onRenameTab={handleOpenRenameDialogForSession}
        onKillTab={setKillDialogSession}
        onSelectTab={activateSession}
        onCloseTab={closeTab}
        onReorderTabs={reorderTabs}
        onSendKey={sendKey}
        onFlushComposition={flushComposition}
        onFocusTerminal={focusTerminal}
        onZoomIn={zoomIn}
        onZoomOut={zoomOut}
        onOpenTimeline={() => {
          setTimelineOpen(true)
          void loadTimeline({ quiet: true })
        }}
      />

      <TimelineDialog
        open={timelineOpen}
        onOpenChange={setTimelineOpen}
        loading={timelineLoading}
        error={timelineError}
        events={timelineEvents}
        hasMore={timelineHasMore}
        query={timelineQuery}
        severity={timelineSeverity}
        eventType={timelineEventType}
        sessionFilter={timelineSessionFilter}
        sessionOptions={timelineSessionOptions}
        onQueryChange={setTimelineQuery}
        onSeverityChange={setTimelineSeverity}
        onEventTypeChange={setTimelineEventType}
        onSessionFilterChange={setTimelineSessionFilter}
        onRefresh={() => {
          void loadTimeline()
        }}
      />

      <Dialog
        open={recoveryDialogOpen}
        onOpenChange={(open) => {
          setRecoveryDialogOpen(open)
          if (open) void refreshRecovery()
        }}
      >
        <DialogContent className="max-h-[88vh] overflow-hidden sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>Recovery Center</DialogTitle>
            <DialogDescription>
              Restore tmux sessions interrupted by reboot or power loss.
            </DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 gap-3 md:grid-cols-[15rem_1fr]">
            <section className="grid min-h-0 gap-2 rounded-md border border-border-subtle bg-secondary p-2">
              <div className="flex items-center justify-between">
                <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                  Sessions
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-6 px-2 text-[11px]"
                  type="button"
                  onClick={() => {
                    void refreshRecovery()
                  }}
                  disabled={recoveryLoading}
                >
                  Refresh
                </Button>
              </div>
              <div className="min-h-0 overflow-auto">
                {recoverySessions.length === 0 ? (
                  <p className="px-1 py-2 text-[12px] text-muted-foreground">
                    No recoverable sessions.
                  </p>
                ) : (
                  <ul className="grid gap-1">
                    {recoverySessions.map((item) => (
                      <li key={item.name}>
                        <button
                          type="button"
                          className={`w-full rounded-md border px-2 py-1.5 text-left text-[12px] ${
                            selectedRecoverySession === item.name
                              ? 'border-primary/60 bg-primary/10'
                              : 'border-border-subtle bg-surface-overlay hover:border-border'
                          }`}
                          onClick={() => {
                            setSelectedRecoverySession(item.name)
                            setRestoreTargetSession(item.name)
                          }}
                        >
                          <div className="flex items-center justify-between gap-2">
                            <span className="truncate font-medium">
                              {item.name}
                            </span>
                            <Badge
                              variant={
                                item.state === 'restored'
                                  ? 'secondary'
                                  : item.state === 'restoring'
                                    ? 'outline'
                                    : 'destructive'
                              }
                            >
                              {item.state}
                            </Badge>
                          </div>
                          <div className="mt-1 text-[10px] text-muted-foreground">
                            {item.windows} windows · {item.panes} panes
                          </div>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </section>

            <section className="grid min-h-0 grid-rows-[auto_auto_1fr_auto] gap-2 rounded-md border border-border-subtle bg-secondary p-3">
              <div className="flex items-center gap-2">
                <Badge variant="outline">
                  {selectedRecoverySession ?? 'Select a session'}
                </Badge>
                {recoveryBusy && <Badge variant="outline">Restoring…</Badge>}
              </div>

              <div className="grid gap-2 md:grid-cols-3">
                <div className="grid gap-1">
                  <span className="text-[11px] text-muted-foreground">
                    Snapshot
                  </span>
                  <Select
                    value={selectedSnapshotID ? String(selectedSnapshotID) : ''}
                    onValueChange={(value) => {
                      const id = Number(value)
                      if (Number.isFinite(id) && id > 0) {
                        void loadRecoverySnapshot(id)
                      }
                    }}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder="Choose snapshot" />
                    </SelectTrigger>
                    <SelectContent>
                      {recoverySnapshots.map((item) => (
                        <SelectItem key={item.id} value={String(item.id)}>
                          #{item.id} ·{' '}
                          {new Date(item.capturedAt).toLocaleString()}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-1">
                  <span className="text-[11px] text-muted-foreground">
                    Replay mode
                  </span>
                  <Select
                    value={restoreMode}
                    onValueChange={(value) =>
                      setRestoreMode(value as 'safe' | 'confirm' | 'full')
                    }
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="safe">safe</SelectItem>
                      <SelectItem value="confirm">confirm</SelectItem>
                      <SelectItem value="full">full</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-1">
                  <span className="text-[11px] text-muted-foreground">
                    Name conflict
                  </span>
                  <Select
                    value={restoreConflictPolicy}
                    onValueChange={(value) =>
                      setRestoreConflictPolicy(
                        value as 'rename' | 'replace' | 'skip',
                      )
                    }
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="rename">rename</SelectItem>
                      <SelectItem value="replace">replace</SelectItem>
                      <SelectItem value="skip">skip</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="grid min-h-0 gap-2 overflow-auto rounded-md border border-border-subtle bg-surface-overlay p-2">
                <div className="grid gap-1">
                  <span className="text-[11px] text-muted-foreground">
                    Target session
                  </span>
                  <Input
                    value={restoreTargetSession}
                    onChange={(event) =>
                      setRestoreTargetSession(
                        slugifyTmuxName(event.target.value),
                      )
                    }
                    placeholder="restored session name"
                  />
                </div>
                {selectedSnapshot ? (
                  <div className="grid gap-2 text-[12px]">
                    <div className="text-muted-foreground">
                      Captured:{' '}
                      {new Date(
                        selectedSnapshot.payload.capturedAt,
                      ).toLocaleString()}
                    </div>
                    <div className="text-muted-foreground">
                      {selectedSnapshot.payload.windows.length} windows ·{' '}
                      {selectedSnapshot.payload.panes.length} panes
                    </div>
                    <div className="grid gap-1">
                      <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                        Windows
                      </span>
                      <div className="max-h-24 overflow-auto rounded border border-border-subtle bg-secondary p-1 text-[11px]">
                        {selectedSnapshot.payload.windows.map((window) => (
                          <div key={`${window.index}-${window.name}`}>
                            #{window.index} {window.name} ({window.panes} panes)
                          </div>
                        ))}
                      </div>
                    </div>
                    <div className="grid gap-1">
                      <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                        Panes
                      </span>
                      <div className="max-h-28 overflow-auto rounded border border-border-subtle bg-secondary p-1 text-[11px]">
                        {selectedSnapshot.payload.panes.map((pane) => (
                          <div
                            key={`${pane.windowIndex}-${pane.paneIndex}-${pane.title}`}
                          >
                            {pane.windowIndex}.{pane.paneIndex} ·{' '}
                            {pane.currentPath || '~'}
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                ) : (
                  <p className="text-[12px] text-muted-foreground">
                    Select a snapshot to inspect and restore.
                  </p>
                )}
              </div>

              <DialogFooter className="items-center justify-between">
                <div className="min-h-[1.25rem] text-[11px] text-destructive-foreground">
                  {recoveryError}
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    type="button"
                    onClick={() => {
                      if (selectedRecoverySession) {
                        void archiveRecoverySession(selectedRecoverySession)
                      }
                    }}
                    disabled={selectedRecoverySession === null || recoveryBusy}
                  >
                    Archive
                  </Button>
                  <Button
                    type="button"
                    onClick={() => {
                      void restoreSelectedSnapshot()
                    }}
                    disabled={selectedSnapshotID === null || recoveryBusy}
                  >
                    Restore Snapshot
                  </Button>
                </div>
              </DialogFooter>
            </section>
          </div>

          {recoveryJobs.length > 0 && (
            <div className="mt-2 rounded-md border border-border-subtle bg-surface-overlay p-2 text-[11px]">
              <p className="font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Recent Jobs
              </p>
              <div className="mt-1 grid gap-1">
                {recoveryJobs.slice(0, 6).map((job) => (
                  <div
                    key={job.id}
                    className="flex items-center justify-between gap-2"
                  >
                    <span className="truncate">
                      {job.sessionName} → {job.targetSession || job.sessionName}
                    </span>
                    <span className="tabular-nums text-muted-foreground">
                      {job.completedSteps}/{job.totalSteps} · {job.status}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={killDialogSession !== null}
        onOpenChange={(open) => {
          if (!open) setKillDialogSession(null)
        }}
      >
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>Kill session</AlertDialogTitle>
            <AlertDialogDescription>
              Kill session {killDialogSession}? This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleConfirmKill}
            >
              Kill
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={renameDialogOpen}
        onOpenChange={(open) => {
          setRenameDialogOpen(open)
          if (!open) setRenameSessionTarget(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename session</DialogTitle>
            <DialogDescription>
              Enter a new name for the active session.
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              handleSubmitRename()
            }}
          >
            <Input
              value={renameValue}
              onChange={(e) => setRenameValue(slugifyTmuxName(e.target.value))}
              autoFocus
            />
            <DialogFooter className="mt-4">
              <DialogClose asChild>
                <Button variant="outline">Cancel</Button>
              </DialogClose>
              <Button type="submit">Rename</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={renameWindowDialogOpen}
        onOpenChange={(open) => {
          setRenameWindowDialogOpen(open)
          if (!open) setRenameWindowIndex(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename window</DialogTitle>
            <DialogDescription>
              Enter a new name for this tmux window.
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              handleSubmitRenameWindow()
            }}
          >
            <Input
              value={renameWindowValue}
              onChange={(e) =>
                setRenameWindowValue(slugifyTmuxName(e.target.value))
              }
              autoFocus
            />
            <DialogFooter className="mt-4">
              <DialogClose asChild>
                <Button variant="outline">Cancel</Button>
              </DialogClose>
              <Button type="submit">Rename</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={renamePaneDialogOpen}
        onOpenChange={(open) => {
          setRenamePaneDialogOpen(open)
          if (!open) setRenamePaneID(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename pane</DialogTitle>
            <DialogDescription>
              Enter a new title for this tmux pane.
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              handleSubmitRenamePane()
            }}
          >
            <Input
              value={renamePaneValue}
              onChange={(e) =>
                setRenamePaneValue(slugifyTmuxName(e.target.value))
              }
              autoFocus
            />
            <DialogFooter className="mt-4">
              <DialogClose asChild>
                <Button variant="outline">Cancel</Button>
              </DialogClose>
              <Button type="submit">Rename</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AppShell>
  )
}

export const Route = createFileRoute('/tmux')({
  component: TmuxPage,
})
