import { useCallback, useEffect, useRef, useState } from 'react'
import { GuardrailConfirmError } from './useTmuxApi'
import { isTmuxBinaryMissingMessage } from './tmuxTypes'
import type { ConnectionState, Session, SessionsResponse } from '@/types'
import type {
  ApiFunction,
  DispatchTabs,
  RuntimeMetrics,
  TmuxSessionsUpdatedPayload,
  TabsStateRef,
} from './tmuxTypes'
import { slugifyTmuxName } from '@/lib/tmuxName'
import { createTmuxOperationId } from '@/lib/tmuxOperationId'
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
    options?: { background?: boolean; force?: boolean },
  ) => Promise<void>
  clearPendingInspectorSessionState: (session: string) => void
  pushErrorToast: (title: string, message: string) => void
  pushSuccessToast: (title: string, message: string) => void
  pendingCreateSessionsRef: React.MutableRefObject<Map<string, string>>
  requestGuardrailConfirm: (
    ruleName: string,
    message: string,
    onConfirm: () => void,
  ) => void
  refreshSessionPresets: () => Promise<void> | void
}

type PendingSessionCreateOperation = {
  operationId: string
  sessionName: string
  optimisticSessionName: string | null
  previousActiveSession: string
  eventSeen: boolean
  converged: boolean
  timeoutId: number | null
}

const sessionCreateConvergenceTimeoutMs = 4_000

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
    requestGuardrailConfirm,
    refreshSessionPresets,
  } = options

  const refreshGenerationRef = useRef(0)
  const pendingKillSessionsRef = useRef(new Set<string>())
  const pendingRenameSessionsRef = useRef(new Map<string, string>())
  const pendingSessionCreateOpsRef = useRef(
    new Map<string, PendingSessionCreateOperation>(),
  )
  const lastSessionsRefreshAtRef = useRef(0)
  const refreshSessionsFnRef = useRef<() => Promise<void>>(async () => {})

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

  const clearSessionCreateTimeout = useCallback((operationId: string) => {
    const op = pendingSessionCreateOpsRef.current.get(operationId)
    if (!op || op.timeoutId === null) {
      return
    }
    window.clearTimeout(op.timeoutId)
    op.timeoutId = null
  }, [])

  const settlePendingSessionCreateSuccess = useCallback(
    (operationId: string) => {
      const op = pendingSessionCreateOpsRef.current.get(operationId)
      if (!op) {
        return
      }
      clearSessionCreateTimeout(operationId)
      pendingSessionCreateOpsRef.current.delete(operationId)
      pushSuccessToast('Create Session', `session "${op.sessionName}" created`)
    },
    [clearSessionCreateTimeout, pushSuccessToast],
  )

  const rollbackPendingSessionCreate = useCallback(
    (operationId: string, message: string) => {
      const op = pendingSessionCreateOpsRef.current.get(operationId)
      if (!op) {
        return
      }
      clearSessionCreateTimeout(operationId)
      pendingSessionCreateOpsRef.current.delete(operationId)
      pendingCreateSessionsRef.current.delete(op.sessionName)
      clearPendingInspectorSessionState(op.sessionName)
      clearPendingSessionRenamesForName(op.sessionName)

      const optimisticSessionName = op.optimisticSessionName?.trim() ?? ''
      if (optimisticSessionName !== '') {
        setSessions((prev) =>
          prev.filter((item) => item.name !== optimisticSessionName),
        )
        dispatchTabs({ type: 'close', session: optimisticSessionName })
        const currentActiveSession = tabsStateRef.current.activeSession
        if (
          currentActiveSession === optimisticSessionName &&
          op.previousActiveSession !== '' &&
          op.previousActiveSession !== optimisticSessionName
        ) {
          dispatchTabs({
            type: 'activate',
            session: op.previousActiveSession,
          })
        }
      }

      setConnection('error', message)
      pushErrorToast('Create Session', message)
    },
    [
      clearPendingInspectorSessionState,
      clearPendingSessionRenamesForName,
      clearSessionCreateTimeout,
      dispatchTabs,
      pendingCreateSessionsRef,
      pushErrorToast,
      setConnection,
      setSessions,
      tabsStateRef,
    ],
  )

  const settlePendingSessionCreateIfReady = useCallback(
    (operationId: string) => {
      const op = pendingSessionCreateOpsRef.current.get(operationId)
      if (!op || !op.eventSeen || !op.converged) {
        return
      }
      settlePendingSessionCreateSuccess(operationId)
    },
    [settlePendingSessionCreateSuccess],
  )

  const markPendingSessionCreateConverged = useCallback(
    (sessionName: string) => {
      const target = sessionName.trim()
      if (target === '') {
        return
      }
      for (const [operationId, op] of pendingSessionCreateOpsRef.current) {
        if (op.sessionName !== target) {
          continue
        }
        op.converged = true
        settlePendingSessionCreateIfReady(operationId)
      }
    },
    [settlePendingSessionCreateIfReady],
  )

  const armPendingSessionCreateTimeout = useCallback(
    (operationId: string) => {
      const op = pendingSessionCreateOpsRef.current.get(operationId)
      if (!op || op.timeoutId !== null) {
        return
      }
      op.timeoutId = window.setTimeout(() => {
        void (async () => {
          const currentOp = pendingSessionCreateOpsRef.current.get(operationId)
          if (!currentOp) {
            return
          }
          await refreshSessionsFnRef.current()
          if (currentOp.sessionName.trim() !== '') {
            await refreshInspector(currentOp.sessionName, { force: true })
          }
          const refreshedOp =
            pendingSessionCreateOpsRef.current.get(operationId)
          if (!refreshedOp) {
            return
          }
          if (refreshedOp.converged) {
            settlePendingSessionCreateSuccess(operationId)
            return
          }
          rollbackPendingSessionCreate(
            operationId,
            `timed out waiting for session "${refreshedOp.sessionName}" to be ready`,
          )
        })()
      }, sessionCreateConvergenceTimeoutMs)
    },
    [
      refreshInspector,
      rollbackPendingSessionCreate,
      settlePendingSessionCreateSuccess,
    ],
  )

  useEffect(() => {
    return () => {
      for (const operationId of pendingSessionCreateOpsRef.current.keys()) {
        clearSessionCreateTimeout(operationId)
      }
      pendingSessionCreateOpsRef.current.clear()
    }
  }, [clearSessionCreateTimeout])

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
      // Pending creates are cleared only after the sessions endpoint
      // confirms them. This keeps optimistic sidebar entries visible while
      // tmux/watchtower convergence is still catching up after creation.
      for (const name of merged.confirmedPendingNames) {
        markPendingSessionCreateConverged(name)
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
    markPendingSessionCreateConverged,
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
  refreshSessionsFnRef.current = refreshSessions

  const activateSession = useCallback(
    (session: string, icon = '') => {
      const optimisticAt = new Date().toISOString()
      setSessions((prev) =>
        upsertOptimisticAttachedSession(prev, session, optimisticAt, icon),
      )
      dispatchTabs({ type: 'activate', session })
    },
    [dispatchTabs, setSessions],
  )

  const createSessionWithConfirm = useCallback(
    async (
      name: string,
      cwd: string,
      icon: string,
      guardrailConfirmed: boolean,
      user?: string,
    ) => {
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
      const operationId = createTmuxOperationId('session-create')
      const operation: PendingSessionCreateOperation = {
        operationId,
        sessionName,
        optimisticSessionName: null,
        previousActiveSession,
        eventSeen: false,
        converged: false,
        timeoutId: null,
      }
      pendingSessionCreateOpsRef.current.set(operationId, operation)
      if (!sessionAlreadyExists) {
        const optimisticAt = new Date().toISOString()
        pendingCreateSessionsRef.current.set(sessionName, optimisticAt)
        operation.optimisticSessionName = sessionName
        setSessions((prev) =>
          upsertOptimisticAttachedSession(
            prev,
            sessionName,
            optimisticAt,
            icon,
            user,
          ),
        )
        dispatchTabs({ type: 'activate', session: sessionName })
        setConnection('connecting', `creating ${sessionName}`)
      }

      try {
        const headers: Record<string, string> = {}
        if (guardrailConfirmed) {
          headers['X-Sentinel-Guardrail-Confirm'] = 'true'
        }
        const body: Record<string, string> = {
          name: sessionName,
          cwd,
          icon,
          operationId,
        }
        if (user) body.user = user
        const result = await api<{ name: string }>('/api/tmux/sessions', {
          method: 'POST',
          body: JSON.stringify(body),
          headers,
        })

        // The server may return a suffixed name (e.g., "dev-1") if the
        // original name was already taken.
        const createdName = result.name || sessionName
        if (createdName !== sessionName) {
          operation.sessionName = createdName
          pendingCreateSessionsRef.current.delete(sessionName)
          pendingCreateSessionsRef.current.set(
            createdName,
            new Date().toISOString(),
          )
          if (!sessionAlreadyExists) {
            setSessions((prev) => {
              const without = prev.filter((s) => s.name !== sessionName)
              return upsertOptimisticAttachedSession(
                without,
                createdName,
                new Date().toISOString(),
                icon,
                user,
              )
            })
            operation.optimisticSessionName = createdName
            dispatchTabs({ type: 'close', session: sessionName })
            dispatchTabs({ type: 'activate', session: createdName })
          } else {
            setSessions((prev) =>
              upsertOptimisticAttachedSession(
                prev,
                createdName,
                new Date().toISOString(),
                icon,
                user,
              ),
            )
            operation.optimisticSessionName = createdName
            dispatchTabs({ type: 'activate', session: createdName })
          }
        } else if (sessionAlreadyExists) {
          activateSession(createdName, icon)
        }
        setConnection('connecting', `opening ${createdName}`)
        armPendingSessionCreateTimeout(operationId)
        settlePendingSessionCreateIfReady(operationId)
      } catch (error) {
        if (error instanceof GuardrailConfirmError) {
          pendingSessionCreateOpsRef.current.delete(operationId)
          if (operation.optimisticSessionName !== null) {
            pendingCreateSessionsRef.current.delete(operation.sessionName)
            setSessions((prev) =>
              prev.filter(
                (item) => item.name !== operation.optimisticSessionName,
              ),
            )
            dispatchTabs({
              type: 'close',
              session: operation.optimisticSessionName,
            })
            const currentActiveSession = tabsStateRef.current.activeSession
            if (
              currentActiveSession === operation.optimisticSessionName &&
              previousActiveSession !== '' &&
              previousActiveSession !== operation.optimisticSessionName
            ) {
              dispatchTabs({
                type: 'activate',
                session: previousActiveSession,
              })
            }
          }
          const rules = error.decision.matchedRules
          requestGuardrailConfirm(
            rules[0]?.name ?? '',
            error.decision.message,
            () =>
              void createSessionWithConfirm(sessionName, cwd, icon, true, user),
          )
          return
        }

        const msg =
          error instanceof Error ? error.message : 'failed to create session'
        rollbackPendingSessionCreate(operationId, msg)
      }
    },
    [
      activateSession,
      api,
      armPendingSessionCreateTimeout,
      clearPendingInspectorSessionState,
      clearPendingSessionRenamesForName,
      dispatchTabs,
      pendingCreateSessionsRef,
      rollbackPendingSessionCreate,
      settlePendingSessionCreateIfReady,
      pushErrorToast,
      requestGuardrailConfirm,
      sessionsRef,
      setConnection,
      setSessions,
      tabsStateRef,
    ],
  )

  const createSession = useCallback(
    async (name: string, cwd: string, icon = '', user?: string) => {
      await createSessionWithConfirm(name, cwd, icon, false, user)
    },
    [createSessionWithConfirm],
  )

  const handleTmuxSessionsEvent = useCallback(
    (payload: TmuxSessionsUpdatedPayload | undefined) => {
      const operationId = payload?.operationId?.trim() ?? ''
      const action = payload?.action?.trim().toLowerCase() ?? ''
      if (operationId === '' || action !== 'create') {
        return false
      }
      const op = pendingSessionCreateOpsRef.current.get(operationId)
      if (!op) {
        return false
      }
      op.eventSeen = true
      const eventSessionName = payload?.session?.trim() ?? ''
      if (eventSessionName !== '') {
        if (eventSessionName !== op.sessionName) {
          const pendingCreatedAt =
            pendingCreateSessionsRef.current.get(op.sessionName) ??
            new Date().toISOString()
          pendingCreateSessionsRef.current.delete(op.sessionName)
          pendingCreateSessionsRef.current.set(
            eventSessionName,
            pendingCreatedAt,
          )
          if (
            op.optimisticSessionName !== null &&
            op.optimisticSessionName !== eventSessionName
          ) {
            setSessions((prev) => {
              const previousOptimistic = prev.find(
                (item) => item.name === op.optimisticSessionName,
              )
              const withoutPrevious = prev.filter(
                (item) => item.name !== op.optimisticSessionName,
              )
              return upsertOptimisticAttachedSession(
                withoutPrevious,
                eventSessionName,
                pendingCreatedAt,
                previousOptimistic?.icon ?? '',
                previousOptimistic?.user,
              )
            })
            dispatchTabs({ type: 'close', session: op.optimisticSessionName })
            dispatchTabs({ type: 'activate', session: eventSessionName })
            op.optimisticSessionName = eventSessionName
          }
        }
        op.sessionName = eventSessionName
      }
      const targetSession = op.sessionName
      if (targetSession !== '') {
        void refreshInspector(targetSession, { force: true })
      }
      void refreshSessions()
      settlePendingSessionCreateIfReady(operationId)
      return true
    },
    [
      dispatchTabs,
      pendingCreateSessionsRef,
      refreshInspector,
      refreshSessions,
      setSessions,
      settlePendingSessionCreateIfReady,
    ],
  )

  const applyKillOptimisticUI = useCallback(
    (sessionName: string) => {
      const wasActive = tabsStateRef.current.activeSession === sessionName
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
    },
    [
      clearPendingInspectorSessionState,
      clearPendingSessionRenamesForName,
      closeCurrentSocket,
      dispatchTabs,
      pendingCreateSessionsRef,
      resetTerminal,
      sessionsRef,
      setConnection,
      setSessions,
      tabsStateRef,
    ],
  )

  const killSessionWithConfirm = useCallback(
    async (name: string, guardrailConfirmed: boolean) => {
      const sessionName = name.trim()
      if (sessionName === '') {
        return
      }

      const activeBeforeKill = tabsStateRef.current.activeSession
      const hadSession = sessionsRef.current.some(
        (item) => item.name === sessionName,
      )

      // Apply optimistic UI only when confirmed (or when guardrails
      // won't intervene). On the initial attempt we wait for the API
      // response so a guardrail rejection doesn't cause a flash.
      if (guardrailConfirmed) {
        applyKillOptimisticUI(sessionName)
      }

      try {
        const killURL = `/api/tmux/sessions/${encodeURIComponent(sessionName)}`
        const headers: Record<string, string> = {}
        if (guardrailConfirmed) {
          headers['X-Sentinel-Guardrail-Confirm'] = 'true'
        }
        await api<void>(killURL, {
          method: 'DELETE',
          headers,
        })

        // API succeeded — apply optimistic UI now if we deferred it.
        if (!guardrailConfirmed) {
          applyKillOptimisticUI(sessionName)
        }
        void refreshSessions()
        void refreshSessionPresets()
        pushSuccessToast('Kill Session', `session "${sessionName}" killed`)
      } catch (error) {
        if (error instanceof GuardrailConfirmError) {
          const rules = error.decision.matchedRules
          requestGuardrailConfirm(
            rules[0]?.name ?? '',
            error.decision.message,
            () => void killSessionWithConfirm(sessionName, true),
          )
          return
        }

        pendingKillSessionsRef.current.delete(sessionName)
        if (guardrailConfirmed) {
          if (hadSession) {
            void refreshSessions()
          }
          if (activeBeforeKill !== '') {
            dispatchTabs({ type: 'activate', session: activeBeforeKill })
          }
        }

        const msg =
          error instanceof Error ? error.message : 'failed to kill session'
        setConnection('error', msg)
        pushErrorToast('Kill Session', msg)
      }
    },
    [
      api,
      applyKillOptimisticUI,
      dispatchTabs,
      pushErrorToast,
      pushSuccessToast,
      refreshSessions,
      requestGuardrailConfirm,
      sessionsRef,
      setConnection,
      tabsStateRef,
    ],
  )

  const killSession = useCallback(
    async (name: string) => {
      await killSessionWithConfirm(name, false)
    },
    [killSessionWithConfirm],
  )

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
          {
            method: 'PATCH',
            body: JSON.stringify({ newName: sanitized }),
          },
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
    handleTmuxSessionsEvent,
    killSession,
    renameActive,
    setSessionIcon,
    handleOpenRenameDialogForSession,
    handleSubmitRename,
    closeTab,
    detachSession,
    reorderTabs,
    setRenameDialogOpen,
    setRenameSessionTarget,
    setRenameValue,
    clearPendingSessionRenamesForName,
  }
}
