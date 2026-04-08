import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  ActivityDeltaResponse,
  ApiFunction,
  InspectorSessionPatch,
  PresenceSocketRef,
  RuntimeMetrics,
  SeenAckMessage,
  TmuxInspectorUpdatedPayload,
  TmuxSessionsUpdatedPayload,
  TabsStateRef,
} from './tmuxTypes'
import type {
  SessionActivityPatch,
  SessionPatchApplyResult,
} from '@/lib/tmuxSessionEvents'
import { shouldRefreshSessionsFromEvent } from '@/lib/tmuxSessionEvents'
import { shouldRefreshTimelineFromEvent } from '@/lib/tmuxTimeline'
import { buildWSProtocols } from '@/lib/wsAuth'

type UseTmuxEventsSocketOptions = {
  api: ApiFunction
  authenticated: boolean
  tokenRequired: boolean
  setToken: (token: string) => void
  presenceSocketRef: PresenceSocketRef
  tabsStateRef: TabsStateRef
  eventsSocketConnectedRef: React.MutableRefObject<boolean>
  runtimeMetricsRef: React.MutableRefObject<RuntimeMetrics>
  lastSessionsRefreshAtRef: React.MutableRefObject<number>
  sendPresenceOverWS: (force: boolean) => boolean
  refreshSessions: () => Promise<void>
  refreshInspector: (
    target: string,
    options?: { background?: boolean; force?: boolean },
  ) => Promise<void>
  pushErrorToast: (title: string, message: string) => void
  applySessionActivityPatches: (
    rawPatches: Array<SessionActivityPatch> | undefined,
  ) => SessionPatchApplyResult
  applyInspectorProjectionPatches: (
    rawPatches: Array<InspectorSessionPatch> | undefined,
  ) => boolean
  settlePendingSeenAcks: (ok: boolean) => void
  seenAckWaitersRef: React.MutableRefObject<Map<string, (ok: boolean) => void>>
  timelineOpenRef: React.MutableRefObject<boolean>
  timelineSessionFilterRef: React.MutableRefObject<string>
  loadTimelineRef: React.MutableRefObject<
    (options?: { quiet?: boolean }) => void
  >
  handleTmuxSessionsEvent?: (
    payload: TmuxSessionsUpdatedPayload | undefined,
  ) => boolean
  handleTmuxInspectorEvent?: (
    payload: TmuxInspectorUpdatedPayload | undefined,
  ) => boolean
}

export function useTmuxEventsSocket(options: UseTmuxEventsSocketOptions) {
  const {
    api,
    authenticated,
    tokenRequired,
    setToken,
    presenceSocketRef,
    tabsStateRef,
    eventsSocketConnectedRef,
    runtimeMetricsRef,
    lastSessionsRefreshAtRef,
    sendPresenceOverWS,
    refreshSessions,
    refreshInspector,
    pushErrorToast,
    applySessionActivityPatches,
    applyInspectorProjectionPatches,
    settlePendingSeenAcks,
    seenAckWaitersRef,
    timelineOpenRef,
    timelineSessionFilterRef,
    loadTimelineRef,
    handleTmuxSessionsEvent,
    handleTmuxInspectorEvent,
  } = options

  const [eventsSocketConnected, setEventsSocketConnected] = useState(false)
  const [reconnectEpoch, setReconnectEpoch] = useState(0)
  const lastGlobalRevRef = useRef(0)
  const lastEventIDRef = useRef(0)
  const lastDeltaSyncAtRef = useRef(0)
  const deltaSyncInFlightRef = useRef(false)
  const wsReconnectAttemptsRef = useRef(0)
  const socketRef = useRef<WebSocket | null>(null)
  const retryTimerRef = useRef<number>(0)
  const refreshTimerRef = useRef<{
    sessions: number | null
    inspector: number | null
    timeline: number | null
  }>({ sessions: null, inspector: null, timeline: null })

  useEffect(() => {
    eventsSocketConnectedRef.current = eventsSocketConnected
  }, [eventsSocketConnected, eventsSocketConnectedRef])

  const syncActivityDelta = useCallback(
    async (params?: { reason?: string; force?: boolean }) => {
      if (tokenRequired && !authenticated) {
        return
      }
      if (deltaSyncInFlightRef.current) {
        return
      }
      if (!params?.force && !eventsSocketConnectedRef.current) {
        return
      }
      const now = Date.now()
      if (now - lastDeltaSyncAtRef.current < 900) {
        return
      }

      deltaSyncInFlightRef.current = true
      lastDeltaSyncAtRef.current = now
      runtimeMetricsRef.current.deltaSyncCount += 1
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
          runtimeMetricsRef.current.deltaOverflowCount += 1
          void refreshSessions()
          const active = tabsStateRef.current.activeSession.trim()
          if (active !== '') {
            void refreshInspector(active, { background: true })
          }
        }
      } catch {
        runtimeMetricsRef.current.deltaSyncErrors += 1
      } finally {
        deltaSyncInFlightRef.current = false
      }
    },
    [
      api,
      applyInspectorProjectionPatches,
      applySessionActivityPatches,
      refreshInspector,
      refreshSessions,
      runtimeMetricsRef,
      tabsStateRef,
      authenticated,
      tokenRequired,
    ],
  )

  const refreshAllState = useCallback(() => {
    if (tokenRequired && !authenticated) {
      return
    }
    void refreshSessions()
    const active = tabsStateRef.current.activeSession.trim()
    if (active !== '') {
      void refreshInspector(active)
    }
  }, [
    refreshInspector,
    refreshSessions,
    tabsStateRef,
    authenticated,
    tokenRequired,
  ])

  // Initial sync on page load
  useEffect(() => {
    refreshAllState()
  }, [refreshAllState])

  // Visibility / online reconciliation
  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        if (eventsSocketConnected) {
          void syncActivityDelta({ reason: 'visibility-visible' })
          return
        }
        refreshAllState()
      }
    }
    const onOnline = () => {
      if (eventsSocketConnected) {
        void syncActivityDelta({ reason: 'browser-online' })
        return
      }
      refreshAllState()
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', onOnline)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', onOnline)
    }
  }, [eventsSocketConnected, refreshAllState, syncActivityDelta])

  // Adaptive fallback: poll only while WS events channel is disconnected
  useEffect(() => {
    if (tokenRequired && !authenticated) return
    if (eventsSocketConnected) return
    runtimeMetricsRef.current.fallbackRefreshCount += 1
    refreshAllState()
    const id = window.setInterval(() => {
      runtimeMetricsRef.current.fallbackRefreshCount += 1
      refreshAllState()
    }, 8_000)
    return () => {
      window.clearInterval(id)
    }
  }, [
    eventsSocketConnected,
    refreshAllState,
    runtimeMetricsRef,
    authenticated,
    tokenRequired,
  ])

  // Expose runtime metrics on window
  useEffect(() => {
    ;(
      window as typeof window & { __SENTINEL_TMUX_METRICS?: unknown }
    ).__SENTINEL_TMUX_METRICS = runtimeMetricsRef.current
    return () => {
      ;(
        window as typeof window & { __SENTINEL_TMUX_METRICS?: unknown }
      ).__SENTINEL_TMUX_METRICS = undefined
    }
  }, [runtimeMetricsRef])

  // Main WebSocket connection effect
  useEffect(() => {
    if (tokenRequired && !authenticated) {
      settlePendingSeenAcks(false)
      presenceSocketRef.current = null
      setEventsSocketConnected(false)
      return
    }

    let closed = false

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
      kind: 'sessions' | 'inspector' | 'timeline',
      scheduleOptions?: { minGapMs?: number },
    ) => {
      if (refreshTimerRef.current[kind] !== null) return
      let delay = 180
      if (kind === 'sessions' && (scheduleOptions?.minGapMs ?? 0) > 0) {
        const elapsed = Date.now() - lastSessionsRefreshAtRef.current
        const gap = scheduleOptions?.minGapMs ?? 0
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
        loadTimelineRef.current({ quiet: true })
      }, delay)
    }

    const connect = () => {
      const wsURL = new URL('/ws/events', window.location.origin)
      const socket = new WebSocket(
        wsURL.toString().replace(/^http/, 'ws'),
        buildWSProtocols(),
      )
      socketRef.current = socket

      socket.onopen = () => {
        runtimeMetricsRef.current.wsOpenCount += 1
        wsReconnectAttemptsRef.current = 0
        presenceSocketRef.current = socket
        setEventsSocketConnected(true)
        sendPresenceOverWS(true)
        void syncActivityDelta({ reason: 'events-open', force: true })
      }

      socket.onmessage = (event) => {
        runtimeMetricsRef.current.wsMessages += 1
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
              const handledTmuxSessionsEvent =
                handleTmuxSessionsEvent?.(msg.payload) === true
              const decision = shouldRefreshSessionsFromEvent(
                msg.payload?.action,
                applySessionActivityPatches(msg.payload?.sessionPatches),
              )
              if (!handledTmuxSessionsEvent && decision.refresh) {
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
              const handledTmuxInspectorEvent =
                handleTmuxInspectorEvent?.(msg.payload) === true
              const target = msg.payload?.session?.trim() ?? ''
              const active = tabsStateRef.current.activeSession
              const skipInspectorRefresh =
                action === 'seen' ||
                action === 'select-window' ||
                action === 'select-pane'
              if (
                !handledTmuxInspectorEvent &&
                !skipInspectorRefresh &&
                target !== '' &&
                target === active
              ) {
                schedule('inspector')
              }
              if (hasEventGap) {
                schedule('sessions')
                if (active.trim() !== '') {
                  schedule('inspector')
                }
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
            default:
              break
          }
        } catch {
          // Ignore non-JSON control messages.
        }
      }

      socket.onerror = () => {
        socket.close()
      }

      socket.onclose = () => {
        runtimeMetricsRef.current.wsCloseCount += 1
        settlePendingSeenAcks(false)
        presenceSocketRef.current = null
        socketRef.current = null
        setEventsSocketConnected(false)
        if (closed) return
        wsReconnectAttemptsRef.current += 1
        runtimeMetricsRef.current.wsReconnects += 1
        const attempt = wsReconnectAttemptsRef.current
        const expo = Math.min(8, attempt)
        const baseDelay = Math.min(10_000, 500 * 2 ** expo)
        const jitter = Math.floor(Math.random() * 300)
        retryTimerRef.current = window.setTimeout(() => {
          retryTimerRef.current = 0
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
      if (retryTimerRef.current !== 0) {
        window.clearTimeout(retryTimerRef.current)
        retryTimerRef.current = 0
      }
      for (const key of ['sessions', 'inspector', 'timeline'] as const) {
        const id = refreshTimerRef.current[key]
        if (id !== null) {
          window.clearTimeout(id)
          refreshTimerRef.current[key] = null
        }
      }
      const sock = socketRef.current
      socketRef.current = null
      if (sock !== null) {
        sock.close()
      }
    }
  }, [
    applyInspectorProjectionPatches,
    applySessionActivityPatches,
    lastSessionsRefreshAtRef,
    loadTimelineRef,
    presenceSocketRef,
    pushErrorToast,
    reconnectEpoch,
    refreshInspector,
    refreshSessions,
    runtimeMetricsRef,
    seenAckWaitersRef,
    sendPresenceOverWS,
    handleTmuxInspectorEvent,
    handleTmuxSessionsEvent,
    setToken,
    settlePendingSeenAcks,
    syncActivityDelta,
    tabsStateRef,
    timelineOpenRef,
    timelineSessionFilterRef,
    authenticated,
    tokenRequired,
  ])

  const forceReconnect = useCallback(() => {
    // Clear any pending retry timer
    window.clearTimeout(retryTimerRef.current)
    retryTimerRef.current = 0
    // Reset backoff
    wsReconnectAttemptsRef.current = 0
    // Close current socket if open
    if (socketRef.current) {
      const sock = socketRef.current
      socketRef.current = null
      sock.onclose = null // prevent onclose from scheduling another retry
      sock.close()
    }
    // Bump epoch to trigger the effect to re-run and establish a new connection
    setReconnectEpoch((prev) => prev + 1)
  }, [])

  return {
    eventsSocketConnected,
    syncActivityDelta,
    forceReconnect,
  }
}
