import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
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
  WindowInfo,
  WindowsResponse,
} from '@/types'
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
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTerminalTmux } from '@/hooks/useTerminalTmux'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { slugifyTmuxName } from '@/lib/tmuxName'
import { buildWSProtocols } from '@/lib/wsAuth'
import { initialTabsState, tabsReducer } from '@/tabsReducer'

function isTmuxBinaryMissingMessage(message: string): boolean {
  const normalized = message.trim().toLowerCase()
  return normalized.includes('tmux binary not found')
}

function TmuxPage() {
  const { tokenRequired, defaultCwd } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()

  const [sessions, setSessions] = useState<Array<Session>>([])
  const [tabsState, dispatchTabs] = useReducer(tabsReducer, initialTabsState)
  const [filter, setFilter] = useState('')

  const [windows, setWindows] = useState<Array<WindowInfo>>([])
  const [panes, setPanes] = useState<Array<PaneInfo>>([])
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
  >([])
  const [recoveryJobs, setRecoveryJobs] = useState<Array<RecoveryJob>>([])
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

  const api = useTmuxApi(token)
  const refreshGenerationRef = useRef(0)
  const inspectorGenerationRef = useRef(0)
  const recoveryGenerationRef = useRef(0)
  const tabsStateRef = useRef(tabsState)
  const refreshTimerRef = useRef<{
    sessions: number | null
    inspector: number | null
    recovery: number | null
  }>({ sessions: null, inspector: null, recovery: null })

  useEffect(() => {
    tabsStateRef.current = tabsState
  }, [tabsState])
  useEffect(() => {
    setActiveWindowIndexOverride(null)
    setActivePaneIDOverride(null)
  }, [tabsState.activeSession])

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

  const refreshSessions = useCallback(async () => {
    const gen = ++refreshGenerationRef.current
    try {
      const data = await api<SessionsResponse>('/api/tmux/sessions')
      if (gen !== refreshGenerationRef.current) return
      setTmuxUnavailable(false)
      setSessions(data.sessions)
      const sessionNames = data.sessions.map((s) => s.name)
      const cur = tabsStateRef.current.activeSession
      if (cur !== '' && !sessionNames.includes(cur)) {
        closeCurrentSocket('active session removed')
        resetTerminal()
        setConnection('disconnected', 'active session removed')
      }
      dispatchTabs({ type: 'sync', sessions: sessionNames })
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'failed to refresh sessions'
      const unavailable = isTmuxBinaryMissingMessage(message)
      setTmuxUnavailable(unavailable)
      setConnection('error', message)
    }
  }, [api, closeCurrentSocket, resetTerminal, setConnection])

  const refreshInspector = useCallback(
    async (target: string, options?: { background?: boolean }) => {
      const session = target.trim()
      const bg = options?.background === true
      if (session === '') {
        setWindows([])
        setPanes([])
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
        setInspectorError('')
        setInspectorLoading(false)
        return
      }
      const gen = ++inspectorGenerationRef.current
      if (!bg) setInspectorLoading(true)
      setInspectorError('')
      try {
        const [wData, pData] = await Promise.all([
          api<WindowsResponse>(
            `/api/tmux/sessions/${encodeURIComponent(session)}/windows`,
          ),
          api<PanesResponse>(
            `/api/tmux/sessions/${encodeURIComponent(session)}/panes`,
          ),
        ])
        if (gen !== inspectorGenerationRef.current) return
        setWindows(wData.windows)
        setPanes(pData.panes)
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
      } catch (error) {
        if (gen !== inspectorGenerationRef.current) return
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
        if (gen === inspectorGenerationRef.current && !bg)
          setInspectorLoading(false)
      }
    },
    [api],
  )

  useEffect(() => {
    // Keep inspector in sync when user switches active session/tab.
    void refreshInspector(tabsState.activeSession)
  }, [refreshInspector, tabsState.activeSession])

  const refreshRecovery = useCallback(
    async (options?: { quiet?: boolean }) => {
      const gen = ++recoveryGenerationRef.current
      if (!options?.quiet) {
        setRecoveryLoading(true)
      }
      try {
        const data = await api<RecoveryOverviewResponse>('/api/recovery/overview')
        if (gen !== recoveryGenerationRef.current) return
        setRecoverySessions(data.overview.killedSessions)
        setRecoveryJobs(data.overview.runningJobs)
        setRecoveryError('')
      } catch (error) {
        if (gen !== recoveryGenerationRef.current) return
        const message =
          error instanceof Error ? error.message : 'failed to refresh recovery'
        // Recovery can be disabled by config. Treat this as a non-fatal state.
        if (message.toLowerCase().includes('recovery subsystem is disabled')) {
          setRecoverySessions([])
          setRecoveryJobs([])
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
    [api],
  )

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
          } else if (data.job.status === 'failed' || data.job.status === 'partial') {
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
      if (!name) {
        setConnection('error', 'session name required')
        pushErrorToast('Create Session', 'session name required')
        return
      }
      try {
        await api<{ name: string }>('/api/tmux/sessions', {
          method: 'POST',
          body: JSON.stringify({ name, cwd }),
        })
        await refreshSessions()
        setConnection('disconnected', 'session created')
        pushSuccessToast('Create Session', `session "${name}" created`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to create session'
        setConnection('error', msg)
        pushErrorToast('Create Session', msg)
      }
    },
    [api, pushErrorToast, pushSuccessToast, refreshSessions, setConnection],
  )

  const killSession = useCallback(
    async (name: string) => {
      const wasActive = tabsStateRef.current.activeSession === name
      try {
        await api<void>(`/api/tmux/sessions/${encodeURIComponent(name)}`, {
          method: 'DELETE',
        })
        if (wasActive) {
          closeCurrentSocket('session killed')
          resetTerminal()
        }
        dispatchTabs({ type: 'remove', session: name })
        await refreshSessions()
        if (!wasActive) setConnection('disconnected', 'session killed')
        pushSuccessToast('Kill Session', `session "${name}" killed`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to kill session'
        setConnection('error', msg)
        pushErrorToast('Kill Session', msg)
      }
    },
    [
      api,
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
      try {
        await api<{ name: string }>(
          `/api/tmux/sessions/${encodeURIComponent(active)}`,
          { method: 'PATCH', body: JSON.stringify({ newName: sanitized }) },
        )
        dispatchTabs({ type: 'rename', oldName: active, newName: sanitized })
        await refreshSessions()
        setConnection(
          connectionState === 'connected' ? 'connected' : 'disconnected',
          'session renamed',
        )
        pushSuccessToast('Rename Session', `"${active}" -> "${sanitized}"`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename session'
        setConnection('error', msg)
        pushErrorToast('Rename Session', msg)
      }
    },
    [
      api,
      connectionState,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      setConnection,
    ],
  )

  const setSessionIcon = useCallback(
    async (session: string, icon: string) => {
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(session)}/icon`,
          {
            method: 'PATCH',
            body: JSON.stringify({ icon }),
          },
        )
        setSessions((prev) =>
          prev.map((s) => (s.name === session ? { ...s, icon } : s)),
        )
      } catch {
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
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-window`,
          {
            method: 'POST',
            body: JSON.stringify({ index, name: sanitized }),
          },
        )
        setWindows((prev) =>
          prev.map((w) => (w.index === index ? { ...w, name: sanitized } : w)),
        )
        void refreshInspector(active, { background: true })
        pushSuccessToast('Rename Window', `window #${index} -> "${sanitized}"`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename window'
        setInspectorError(msg)
        pushErrorToast('Rename Window', msg)
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
      const sanitized = slugifyTmuxName(title).trim()
      if (!sanitized) {
        pushErrorToast('Rename Pane', 'pane title required')
        return
      }
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-pane`,
          {
            method: 'POST',
            body: JSON.stringify({ paneId: paneID, title: sanitized }),
          },
        )
        setPanes((prev) =>
          prev.map((p) =>
            p.paneId === paneID ? { ...p, title: sanitized } : p,
          ),
        )
        void refreshInspector(active, { background: true })
        pushSuccessToast('Rename Pane', `pane ${paneID} renamed`)
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename pane'
        setInspectorError(msg)
        pushErrorToast('Rename Pane', msg)
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
      )
        .then(() => {
          void refreshInspector(active, { background: true })
        })
        .catch((error) => {
          const msg =
            error instanceof Error ? error.message : 'failed to switch window'
          setInspectorError(msg)
          pushErrorToast('Switch Window', msg)
          void refreshInspector(active, { background: true })
        })
    },
    [api, panes, pushErrorToast, refreshInspector],
  )

  const selectPane = useCallback(
    (paneID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active || !paneID.trim()) return
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
        void refreshInspector(active, { background: true })
      })
    },
    [api, panes, pushErrorToast, refreshInspector],
  )

  const createWindow = useCallback(() => {
    const active = tabsStateRef.current.activeSession
    if (!active) return
    const nextIdx = windows.reduce((h, w) => Math.max(h, w.index), -1) + 1
    setInspectorError('')
    setWindows((prev) => [
      ...prev.map((w) => ({ ...w, active: false })),
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
        void refreshInspector(active, { background: true })
        void refreshSessions()
      })
      .catch((error) => {
        const msg =
          error instanceof Error ? error.message : 'failed to create window'
        setInspectorError(msg)
        pushErrorToast('New Window', msg)
        void refreshInspector(active, { background: true })
      })
  }, [api, pushErrorToast, refreshInspector, refreshSessions, windows])

  const closeWindow = useCallback(
    (windowIndex: number) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
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
      setWindows(rem.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-window`,
        { method: 'POST', body: JSON.stringify({ index: windowIndex }) },
      )
        .then(() => {
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
        .catch((error) => {
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
      setInspectorError('')
      setWindows(remW.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-pane`,
        { method: 'POST', body: JSON.stringify({ paneId: paneID }) },
      )
        .then(() => {
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
        .catch((error) => {
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
      setInspectorError('')
      setWindows((prev) =>
        prev.map((w) => ({
          ...w,
          active: w.index === target.windowIndex,
          panes: w.index === target.windowIndex ? w.panes + 1 : w.panes,
        })),
      )
      setPanes((prev) =>
        prev.map((p) => ({ ...p, active: p.paneId === targetID })),
      )
      setActiveWindowIndexOverride(target.windowIndex)
      setActivePaneIDOverride(targetID)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/split-pane`,
        {
          method: 'POST',
          body: JSON.stringify({ paneId: targetID, direction }),
        },
      )
        .then(() => {
          void refreshInspector(active, { background: true })
        })
        .catch((error) => {
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

  const refreshAllState = useCallback(
    (options?: { quietRecovery?: boolean }) => {
      void refreshSessions()
      const active = tabsStateRef.current.activeSession.trim()
      if (active !== '') {
        void refreshInspector(active, { background: true })
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
        refreshAllState({ quietRecovery: true })
      }
    }
    const onOnline = () => {
      refreshAllState({ quietRecovery: true })
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', onOnline)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', onOnline)
    }
  }, [refreshAllState])

  useEffect(() => {
    // Adaptive fallback: poll only while WS events channel is disconnected.
    if (eventsSocketConnected) return
    refreshAllState({ quietRecovery: true })
    const id = window.setInterval(() => {
      refreshAllState({ quietRecovery: true })
    }, 8_000)
    return () => {
      window.clearInterval(id)
    }
  }, [eventsSocketConnected, refreshAllState])

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

    const schedule = (kind: 'sessions' | 'inspector' | 'recovery') => {
      if (refreshTimerRef.current[kind] !== null) return
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
        void refreshRecovery({ quiet: true })
      }, 180)
    }

    const connect = () => {
      const wsURL = new URL('/ws/events', window.location.origin)
      socket = new WebSocket(
        wsURL.toString().replace(/^http/, 'ws'),
        buildWSProtocols(token),
      )

      socket.onopen = () => {
        setEventsSocketConnected(true)
        // Reconcile any missed events after reconnect.
        refreshAllState({ quietRecovery: true })
      }

      socket.onmessage = (event) => {
        if (typeof event.data !== 'string') return
        try {
          const msg = JSON.parse(event.data) as {
            type?: string
            payload?: { session?: string }
          }
          switch (msg.type) {
            case 'tmux.sessions.updated':
              schedule('sessions')
              schedule('inspector')
              break
            case 'tmux.inspector.updated': {
              const target = msg.payload?.session?.trim() ?? ''
              const active = tabsStateRef.current.activeSession
              if (target === '' || target === active) {
                schedule('inspector')
              }
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
        setEventsSocketConnected(false)
        if (closed) return
        reconnectTimer = window.setTimeout(() => {
          connect()
        }, 1200)
      }
    }

    connect()

    return () => {
      closed = true
      setEventsSocketConnected(false)
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
      }
      for (const key of ['sessions', 'inspector', 'recovery'] as const) {
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
  }, [refreshAllState, refreshInspector, refreshRecovery, refreshSessions, token])

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
                          #{item.id} · {new Date(item.capturedAt).toLocaleString()}
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
