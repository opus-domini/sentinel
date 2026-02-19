import { useCallback, useRef, useState } from 'react'
import { isTmuxBinaryMissingMessage } from './tmuxTypes'
import type { ConnectionState, Session, SessionsResponse } from '@/types'
import type {
  ApiFunction,
  DispatchTabs,
  RuntimeMetrics,
  TabsStateRef,
} from './tmuxTypes'
import { slugifyTmuxName } from '@/lib/tmuxName'
import {
  mergePendingCreateSessions,
  upsertOptimisticAttachedSession,
} from '@/lib/tmuxSessionCreate'

type UseSessionCRUDOptions = {
  api: ApiFunction
  tabsStateRef: TabsStateRef
  sessionsRef: React.MutableRefObject<Array<Session>>
  runtimeMetricsRef: React.MutableRefObject<RuntimeMetrics>
  dispatchTabs: DispatchTabs
  setSessions: React.Dispatch<React.SetStateAction<Array<Session>>>
  setTmuxUnavailable: (unavailable: boolean) => void
  closeCurrentSocket: (reason: string) => void
  resetTerminal: () => void
  setConnection: (state: ConnectionState, detail: string) => void
  connectionState: ConnectionState
  refreshInspector: (
    target: string,
    options?: { background?: boolean },
  ) => Promise<void>
  clearPendingInspectorSessionState: (session: string) => void
  pushErrorToast: (title: string, message: string) => void
  pushSuccessToast: (title: string, message: string) => void
  pendingCreateSessionsRef: React.MutableRefObject<Map<string, string>>
}

export function useSessionCRUD(options: UseSessionCRUDOptions) {
  const {
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
    refreshInspector,
    clearPendingInspectorSessionState,
    pushErrorToast,
    pushSuccessToast,
    pendingCreateSessionsRef,
  } = options

  const refreshGenerationRef = useRef(0)
  const pendingKillSessionsRef = useRef(new Set<string>())
  const pendingRenameSessionsRef = useRef(new Map<string, string>())
  const lastSessionsRefreshAtRef = useRef(0)

  const [killDialogSession, setKillDialogSession] = useState<string | null>(
    null,
  )
  const [renameDialogOpen, setRenameDialogOpen] = useState(false)
  const [renameSessionTarget, setRenameSessionTarget] = useState<string | null>(
    null,
  )
  const [renameValue, setRenameValue] = useState('')

  const clearPendingSessionRenamesForName = useCallback((session: string) => {
    const name = session.trim()
    if (name === '') return
    for (const [from, to] of pendingRenameSessionsRef.current) {
      if (from === name || to === name) {
        pendingRenameSessionsRef.current.delete(from)
      }
    }
  }, [])

  const refreshSessions = useCallback(async () => {
    runtimeMetricsRef.current.sessionsRefreshCount += 1
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
    clearPendingInspectorSessionState,
    clearPendingSessionRenamesForName,
    closeCurrentSocket,
    dispatchTabs,
    pendingCreateSessionsRef,
    resetTerminal,
    runtimeMetricsRef,
    setConnection,
    setSessions,
    setTmuxUnavailable,
    tabsStateRef,
  ])

  const activateSession = useCallback(
    (session: string) => {
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
    },
    [dispatchTabs, setSessions],
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
        // Clear inspector error for the new session being created
        // (handled by inspector hook's session-switch effect)
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
          dispatchTabs({ type: 'close', session: sessionName })
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
      dispatchTabs,
      pendingCreateSessionsRef,
      pushErrorToast,
      pushSuccessToast,
      refreshInspector,
      refreshSessions,
      sessionsRef,
      setConnection,
      setSessions,
      tabsStateRef,
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
      dispatchTabs({ type: 'close', session: sessionName })
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
      dispatchTabs,
      pendingCreateSessionsRef,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      resetTerminal,
      sessionsRef,
      setConnection,
      setSessions,
      tabsStateRef,
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
      dispatchTabs,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      sessionsRef,
      setConnection,
      setSessions,
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
    [api, pushErrorToast, sessionsRef, setSessions],
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
    [
      closeCurrentSocket,
      dispatchTabs,
      resetTerminal,
      setConnection,
      setSessions,
      tabsStateRef,
    ],
  )

  const detachSession = useCallback(
    (name: string) => {
      const isOpen = tabsStateRef.current.openTabs.includes(name)
      if (!isOpen) return
      closeTab(name)
    },
    [closeTab, tabsStateRef],
  )

  const reorderTabs = useCallback(
    (from: number, to: number) => {
      dispatchTabs({ type: 'reorder', from, to })
    },
    [dispatchTabs],
  )

  // Sync sessions to query cache
  // (This needs to be in sessionCRUD because sessions state & sessionsRef are in TmuxPage)
  // Actually sessions are managed in TmuxPage, but we read sessionsRef from there.
  // The cache sync effect stays in TmuxPage.

  return {
    // State
    killDialogSession,
    renameDialogOpen,
    renameSessionTarget,
    renameValue,
    // Refs
    lastSessionsRefreshAtRef,
    pendingKillSessionsRef,
    pendingRenameSessionsRef,
    // Actions
    refreshSessions,
    activateSession,
    createSession,
    killSession,
    handleConfirmKill,
    renameActive,
    setSessionIcon,
    handleOpenRenameDialogForSession,
    handleSubmitRename,
    closeTab,
    detachSession,
    reorderTabs,
    setKillDialogSession,
    setRenameDialogOpen,
    setRenameSessionTarget,
    setRenameValue,
    clearPendingSessionRenamesForName,
  }
}
