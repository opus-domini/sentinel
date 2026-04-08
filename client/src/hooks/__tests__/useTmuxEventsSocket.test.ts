// @vitest-environment jsdom
import type { MutableRefObject } from 'react'
import type {
  ActivityDeltaResponse,
  ApiFunction,
  InspectorSessionPatch,
  PresenceSocketRef,
  RuntimeMetrics,
  TabsStateRef,
} from '../tmuxTypes'
import type {
  SessionActivityPatch,
  SessionPatchApplyResult,
} from '@/lib/tmuxSessionEvents'
import { act, renderHook } from '@testing-library/react'
import { shouldRefreshSessionsFromEvent } from '@/lib/tmuxSessionEvents'
import { shouldRefreshTimelineFromEvent } from '@/lib/tmuxTimeline'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useTmuxEventsSocket } from '../useTmuxEventsSocket'

// ---------------------------------------------------------------------------
// Mock external modules (vi.mock is hoisted by vitest)
// ---------------------------------------------------------------------------

vi.mock('@/lib/tmuxSessionEvents', () => ({
  shouldRefreshSessionsFromEvent: vi.fn(() => ({
    refresh: false,
  })),
}))

vi.mock('@/lib/tmuxTimeline', () => ({
  shouldRefreshTimelineFromEvent: vi.fn(() => false),
}))

vi.mock('@/lib/wsAuth', () => ({
  buildWSProtocols: () => ['sentinel.v1'],
}))

const mockedShouldRefreshSessions = vi.mocked(shouldRefreshSessionsFromEvent)
const mockedShouldRefreshTimeline = vi.mocked(shouldRefreshTimelineFromEvent)

// ---------------------------------------------------------------------------
// Mock WebSocket
// ---------------------------------------------------------------------------

class MockWebSocket {
  static instances: Array<MockWebSocket> = []

  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  closed = false

  constructor(
    public url: string,
    public protocols?: string | Array<string>,
  ) {
    MockWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
    this.onclose?.(new CloseEvent('close'))
  }

  send() {
    // no-op
  }

  emitOpen() {
    this.onopen?.(new Event('open'))
  }

  emitMessage(data: Record<string, unknown>) {
    this.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify(data) }),
    )
  }

  emitRawMessage(data: string) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  emitClose() {
    this.onclose?.(new CloseEvent('close'))
  }

  emitError() {
    this.onerror?.(new Event('error'))
  }
}

function lastSocket(): MockWebSocket {
  return MockWebSocket.instances[MockWebSocket.instances.length - 1]
}

// ---------------------------------------------------------------------------
// Default runtime metrics factory
// ---------------------------------------------------------------------------

function makeMetrics(): RuntimeMetrics {
  return {
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
  }
}

// ---------------------------------------------------------------------------
// Default options factory
// ---------------------------------------------------------------------------

function makeRef<T>(value: T): MutableRefObject<T> {
  return { current: value }
}

const NO_PATCHES: SessionPatchApplyResult = {
  hasInputPatches: false,
  applied: false,
  hasUnknownSession: false,
}

function defaultDeltaResponse(): ActivityDeltaResponse {
  return {
    globalRev: 1,
    overflow: false,
    sessionPatches: [],
    inspectorPatches: [],
  }
}

type Options = Parameters<typeof useTmuxEventsSocket>[0]

function makeOptions(overrides?: Partial<Options>): Options {
  return {
    api: vi.fn(() =>
      Promise.resolve(defaultDeltaResponse()),
    ) as unknown as ApiFunction,
    authenticated: true,
    tokenRequired: false,
    setToken: vi.fn(),
    presenceSocketRef: makeRef<WebSocket | null>(null) as PresenceSocketRef,
    tabsStateRef: makeRef({
      activeSession: 'main',
      sessions: [],
      activeWindowIndex: null,
      activePaneID: null,
    }) as TabsStateRef,
    eventsSocketConnectedRef: makeRef(false),
    runtimeMetricsRef: makeRef(makeMetrics()),
    lastSessionsRefreshAtRef: makeRef(0),
    sendPresenceOverWS: vi.fn(() => true),
    refreshSessions: vi.fn(() => Promise.resolve()),
    refreshInspector: vi.fn(() => Promise.resolve()),
    pushErrorToast: vi.fn(),
    applySessionActivityPatches: vi.fn(() => NO_PATCHES),
    applyInspectorProjectionPatches: vi.fn(() => false),
    settlePendingSeenAcks: vi.fn(),
    seenAckWaitersRef: makeRef(new Map<string, (ok: boolean) => void>()),
    timelineOpenRef: makeRef(false),
    timelineSessionFilterRef: makeRef('all'),
    loadTimelineRef: makeRef(vi.fn()),
    handleTmuxSessionsEvent: vi.fn(() => false),
    handleTmuxInspectorEvent: vi.fn(() => false),
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useTmuxEventsSocket', () => {
  const originalWebSocket = globalThis.WebSocket

  beforeEach(() => {
    vi.useFakeTimers()
    MockWebSocket.instances = []
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
    mockedShouldRefreshSessions.mockReturnValue({ refresh: false })
    mockedShouldRefreshTimeline.mockReturnValue(false)
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
    vi.restoreAllMocks()
  })

  // -------------------------------------------------------------------------
  // 1. Connection lifecycle
  // -------------------------------------------------------------------------

  describe('connection lifecycle', () => {
    it('connects to WebSocket on mount when authenticated', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1)
      expect(lastSocket().url).toContain('/ws/events')
    })

    it('does not connect when tokenRequired and not authenticated', () => {
      const opts = makeOptions({
        authenticated: false,
        tokenRequired: true,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      expect(MockWebSocket.instances).toHaveLength(0)
    })

    it('connects when tokenRequired is false regardless of authenticated', () => {
      const opts = makeOptions({
        authenticated: false,
        tokenRequired: false,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1)
    })

    it('sets eventsSocketConnected to true on open', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      expect(result.current.eventsSocketConnected).toBe(true)
    })

    it('sets eventsSocketConnected to false on close', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })
      expect(result.current.eventsSocketConnected).toBe(true)

      act(() => {
        lastSocket().emitClose()
      })
      expect(result.current.eventsSocketConnected).toBe(false)
    })

    it('settles pending seen acks when not authenticated', () => {
      const settlePendingSeenAcks = vi.fn()
      const opts = makeOptions({
        authenticated: false,
        tokenRequired: true,
        settlePendingSeenAcks,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      expect(settlePendingSeenAcks).toHaveBeenCalledWith(false)
    })

    it('nulls presenceSocketRef on close', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })
      expect(opts.presenceSocketRef.current).not.toBeNull()

      act(() => {
        lastSocket().emitClose()
      })
      expect(opts.presenceSocketRef.current).toBeNull()
    })

    it('stores socket in presenceSocketRef on open', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      expect(opts.presenceSocketRef.current).toBe(lastSocket())
    })

    it('sends presence over WS on open', () => {
      const sendPresenceOverWS = vi.fn(() => true)
      const opts = makeOptions({ sendPresenceOverWS })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      expect(sendPresenceOverWS).toHaveBeenCalledWith(true)
    })

    it('increments wsOpenCount on open', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      expect(opts.runtimeMetricsRef.current.wsOpenCount).toBe(1)
    })

    it('increments wsCloseCount on close', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
        lastSocket().emitClose()
      })

      expect(opts.runtimeMetricsRef.current.wsCloseCount).toBe(1)
    })

    it('closes socket on unmount', () => {
      const opts = makeOptions()
      const { unmount } = renderHook(() => useTmuxEventsSocket(opts))

      const socket = lastSocket()
      act(() => {
        socket.emitOpen()
      })

      unmount()
      expect(socket.closed).toBe(true)
    })

    it('clears pending refresh timers on unmount', () => {
      mockedShouldRefreshSessions.mockReturnValue({ refresh: true })
      const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout')
      const opts = makeOptions()
      const { unmount } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Schedule a refresh timer so there is something to clear
      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'created' },
        })
      })

      unmount()
      expect(clearTimeoutSpy).toHaveBeenCalled()
    })

    it('reconnects when authentication changes from false to true', () => {
      let authenticated = false
      const opts = makeOptions({ authenticated, tokenRequired: true })

      const { rerender } = renderHook(() =>
        useTmuxEventsSocket({ ...opts, authenticated }),
      )

      expect(MockWebSocket.instances).toHaveLength(0)

      authenticated = true
      rerender()

      expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1)
    })

    it('closes socket on error then triggers onclose', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      const socket = lastSocket()

      act(() => {
        socket.emitError()
      })

      // onerror calls socket.close() which triggers onclose
      expect(socket.closed).toBe(true)
    })
  })

  // -------------------------------------------------------------------------
  // 2. Event dispatching per message type
  // -------------------------------------------------------------------------

  describe('event dispatching', () => {
    it('handles tmux.sessions.updated and calls applySessionActivityPatches', () => {
      const applySessionActivityPatches = vi.fn(() => NO_PATCHES)
      const applyInspectorProjectionPatches = vi.fn(() => false)
      const opts = makeOptions({
        applySessionActivityPatches,
        applyInspectorProjectionPatches,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const sessionPatches: Array<SessionActivityPatch> = [
        { name: 'main', windows: 2 },
      ]
      const inspectorPatches: Array<InspectorSessionPatch> = [
        { session: 'main' },
      ]

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: {
            action: 'created',
            sessionPatches,
            inspectorPatches,
          },
        })
      })

      expect(applySessionActivityPatches).toHaveBeenCalledWith(sessionPatches)
      expect(applyInspectorProjectionPatches).toHaveBeenCalledWith(
        inspectorPatches,
      )
    })

    it('lets a correlated tmux.sessions.updated handler suppress the generic session refresh schedule', () => {
      mockedShouldRefreshSessions.mockReturnValue({ refresh: true })
      const refreshSessions = vi.fn(() => Promise.resolve())
      const handleTmuxSessionsEvent = vi.fn(() => true)
      const opts = makeOptions({
        refreshSessions,
        handleTmuxSessionsEvent,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })
      const refreshSessionsCallsBeforeEvent = refreshSessions.mock.calls.length

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: {
            action: 'create',
            session: 'main',
            operationId: 'session-create-1',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(250)
      })

      expect(handleTmuxSessionsEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          action: 'create',
          session: 'main',
          operationId: 'session-create-1',
        }),
      )
      expect(refreshSessions.mock.calls.length).toBe(
        refreshSessionsCallsBeforeEvent,
      )
    })

    it('handles tmux.activity.updated and applies patches', () => {
      const applySessionActivityPatches = vi.fn(() => NO_PATCHES)
      const applyInspectorProjectionPatches = vi.fn(() => false)
      const opts = makeOptions({
        applySessionActivityPatches,
        applyInspectorProjectionPatches,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.activity.updated',
          eventId: 1,
          payload: {
            sessionPatches: [{ name: 'dev', windows: 1 }],
            inspectorPatches: [{ session: 'dev' }],
          },
        })
      })

      expect(applySessionActivityPatches).toHaveBeenCalledWith([
        { name: 'dev', windows: 1 },
      ])
      expect(applyInspectorProjectionPatches).toHaveBeenCalledWith([
        { session: 'dev' },
      ])
    })

    it('handles tmux.inspector.updated and schedules inspector refresh', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({
        refreshInspector,
        tabsStateRef: makeRef({
          activeSession: 'main',
          sessions: [],
          activeWindowIndex: null,
          activePaneID: null,
        }) as TabsStateRef,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })
      const refreshInspectorCallsBeforeEvent =
        refreshInspector.mock.calls.length

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'resize',
            session: 'main',
          },
        })
      })

      // The refresh is scheduled with a 180ms delay
      act(() => {
        vi.advanceTimersByTime(200)
      })

      expect(refreshInspector).toHaveBeenCalledWith('main', {
        background: true,
      })
    })

    it('lets a correlated tmux.inspector.updated handler suppress the generic inspector refresh schedule', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const handleTmuxInspectorEvent = vi.fn(() => true)
      const opts = makeOptions({
        refreshInspector,
        handleTmuxInspectorEvent,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })
      const refreshInspectorCallsBeforeEvent =
        refreshInspector.mock.calls.length

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'new-window',
            session: 'main',
            operationId: 'window-create-1',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(250)
      })

      expect(handleTmuxInspectorEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          action: 'new-window',
          session: 'main',
          operationId: 'window-create-1',
        }),
      )
      expect(refreshInspector.mock.calls.length).toBe(
        refreshInspectorCallsBeforeEvent,
      )
    })

    it('skips inspector refresh when action is "seen"', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshInspector })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'seen',
            session: 'main',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      // refreshInspector may have been called from the initial sync, but
      // not from this event (since action=seen is skipped)
      const inspectorCallsAfterOpen = refreshInspector.mock.calls.filter(
        (call) => call[1]?.background === true,
      )
      expect(inspectorCallsAfterOpen).toHaveLength(0)
    })

    it('skips inspector refresh when action is "select-window"', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshInspector })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'select-window',
            session: 'main',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      const inspectorCallsAfterOpen = refreshInspector.mock.calls.filter(
        (call) => call[1]?.background === true,
      )
      expect(inspectorCallsAfterOpen).toHaveLength(0)
    })

    it('skips inspector refresh when action is "select-pane"', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshInspector })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'select-pane',
            session: 'main',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      const inspectorCallsAfterOpen = refreshInspector.mock.calls.filter(
        (call) => call[1]?.background === true,
      )
      expect(inspectorCallsAfterOpen).toHaveLength(0)
    })

    it('skips inspector refresh when action is empty', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshInspector })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: '',
            session: 'main',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      const inspectorCallsAfterOpen = refreshInspector.mock.calls.filter(
        (call) => call[1]?.background === true,
      )
      expect(inspectorCallsAfterOpen).toHaveLength(0)
    })

    it('skips inspector refresh when session does not match active', () => {
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({
        refreshInspector,
        tabsStateRef: makeRef({
          activeSession: 'main',
          sessions: [],
          activeWindowIndex: null,
          activePaneID: null,
        }) as TabsStateRef,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: {
            action: 'resize',
            session: 'other-session',
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      const inspectorCallsAfterOpen = refreshInspector.mock.calls.filter(
        (call) => call[1]?.background === true,
      )
      expect(inspectorCallsAfterOpen).toHaveLength(0)
    })

    it('handles tmux.timeline.updated when timeline is open', () => {
      mockedShouldRefreshTimeline.mockReturnValue(true)

      const loadTimeline = vi.fn()
      const opts = makeOptions({
        timelineOpenRef: makeRef(true),
        timelineSessionFilterRef: makeRef('all'),
        loadTimelineRef: makeRef(loadTimeline),
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.timeline.updated',
          eventId: 1,
          payload: {
            sessions: ['main'],
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(200)
      })

      expect(loadTimeline).toHaveBeenCalledWith({ quiet: true })
    })

    it('skips timeline refresh when timeline is closed', () => {
      const loadTimeline = vi.fn()
      const opts = makeOptions({
        timelineOpenRef: makeRef(false),
        loadTimelineRef: makeRef(loadTimeline),
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.timeline.updated',
          eventId: 1,
          payload: {
            sessions: ['main'],
          },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      expect(loadTimeline).not.toHaveBeenCalled()
    })

    it('handles tmux.timeline.updated with session filter "active"', () => {
      mockedShouldRefreshTimeline.mockReturnValue(true)
      const loadTimeline = vi.fn()
      const opts = makeOptions({
        timelineOpenRef: makeRef(true),
        timelineSessionFilterRef: makeRef('active'),
        tabsStateRef: makeRef({
          activeSession: 'dev',
          sessions: [],
          activeWindowIndex: null,
          activePaneID: null,
        }) as TabsStateRef,
        loadTimelineRef: makeRef(loadTimeline),
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.timeline.updated',
          eventId: 1,
          payload: {
            sessions: ['dev'],
          },
        })
      })

      // shouldRefreshTimelineFromEvent is called with sessions and the
      // resolved active session ('dev')
      expect(mockedShouldRefreshTimeline).toHaveBeenCalledWith(['dev'], 'dev')
    })

    it('handles tmux.timeline.updated with specific session filter', () => {
      mockedShouldRefreshTimeline.mockReturnValue(true)
      const loadTimeline = vi.fn()
      const opts = makeOptions({
        timelineOpenRef: makeRef(true),
        timelineSessionFilterRef: makeRef('prod'),
        loadTimelineRef: makeRef(loadTimeline),
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.timeline.updated',
          eventId: 1,
          payload: {
            sessions: ['prod'],
          },
        })
      })

      expect(mockedShouldRefreshTimeline).toHaveBeenCalledWith(['prod'], 'prod')
    })

    it('handles tmux.guardrail.blocked and shows error toast', () => {
      const pushErrorToast = vi.fn()
      const opts = makeOptions({ pushErrorToast })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.guardrail.blocked',
          eventId: 1,
          payload: {
            decision: {
              message: 'rm -rf blocked by policy',
            },
          },
        })
      })

      expect(pushErrorToast).toHaveBeenCalledWith(
        'Guardrail',
        'rm -rf blocked by policy',
      )
    })

    it('uses default guardrail message when decision.message is missing', () => {
      const pushErrorToast = vi.fn()
      const opts = makeOptions({ pushErrorToast })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.guardrail.blocked',
          eventId: 1,
          payload: {},
        })
      })

      expect(pushErrorToast).toHaveBeenCalledWith(
        'Guardrail',
        'Operation blocked by guardrail policy',
      )
    })

    it('handles tmux.auth.expired: clears token and closes socket', () => {
      const setToken = vi.fn()
      const settlePendingSeenAcks = vi.fn()
      const opts = makeOptions({
        setToken,
        tokenRequired: true,
        settlePendingSeenAcks,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      const socket = lastSocket()
      act(() => {
        socket.emitOpen()
      })

      act(() => {
        socket.emitMessage({
          type: 'tmux.auth.expired',
          eventId: 1,
        })
      })

      expect(setToken).toHaveBeenCalledWith('')
      expect(settlePendingSeenAcks).toHaveBeenCalledWith(false)
      expect(socket.closed).toBe(true)
    })

    it('handles tmux.seen.ack and resolves waiter', () => {
      const waiterCallback = vi.fn()
      const seenAckWaitersRef = makeRef(new Map([['req-1', waiterCallback]]))
      const applySessionActivityPatches = vi.fn(() => NO_PATCHES)
      const applyInspectorProjectionPatches = vi.fn(() => false)
      const opts = makeOptions({
        seenAckWaitersRef,
        applySessionActivityPatches,
        applyInspectorProjectionPatches,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const sessionPatches: Array<SessionActivityPatch> = [
        { name: 'main', unreadPanes: 0 },
      ]
      const inspectorPatches: Array<InspectorSessionPatch> = [
        { session: 'main' },
      ]

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.seen.ack',
          requestId: 'req-1',
          eventId: 2,
          sessionPatches,
          inspectorPatches,
        })
      })

      expect(waiterCallback).toHaveBeenCalledWith(true)
      expect(seenAckWaitersRef.current.has('req-1')).toBe(false)
      expect(applySessionActivityPatches).toHaveBeenCalledWith(sessionPatches)
      expect(applyInspectorProjectionPatches).toHaveBeenCalledWith(
        inspectorPatches,
      )
    })

    it('ignores non-string WebSocket messages', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Send a binary-like message (non-string data)
      act(() => {
        lastSocket().onmessage?.(
          new MessageEvent('message', { data: new ArrayBuffer(8) }),
        )
      })

      // Should not throw; wsMessages counter still increments
      expect(opts.runtimeMetricsRef.current.wsMessages).toBe(1)
    })

    it('ignores invalid JSON messages', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitRawMessage('not json')
      })

      // Should not throw
      expect(opts.runtimeMetricsRef.current.wsMessages).toBe(1)
    })

    it('increments wsMessages counter for each message', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: {},
        })
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 2,
          payload: {},
        })
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 3,
          payload: {},
        })
      })

      expect(opts.runtimeMetricsRef.current.wsMessages).toBe(3)
    })
  })

  // -------------------------------------------------------------------------
  // 3. Delta sync debounce and in-flight guard
  // -------------------------------------------------------------------------

  describe('delta sync', () => {
    it('triggers delta sync on socket open', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // syncActivityDelta is called with reason 'events-open' on open
      expect(api).toHaveBeenCalledWith(
        expect.stringContaining('/api/tmux/activity/delta'),
      )
    })

    it('includes since parameter in delta sync request', async () => {
      const api = vi.fn(() =>
        Promise.resolve({ ...defaultDeltaResponse(), globalRev: 5 }),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // First call should be since=0
      expect(api).toHaveBeenCalledWith(expect.stringContaining('since=0'))
    })

    it('debounces delta sync within 900ms window', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      const callCountAfterOpen = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      // Call syncActivityDelta immediately again (within 900ms debounce)
      await act(async () => {
        await result.current.syncActivityDelta({ reason: 'test', force: true })
      })

      const callCountAfterSecond = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      // Should NOT have made another call (900ms debounce)
      expect(callCountAfterSecond).toBe(callCountAfterOpen)
    })

    it('allows delta sync after 900ms debounce period', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      const callCountAfterOpen = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      // Advance past debounce window
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      await act(async () => {
        await result.current.syncActivityDelta({ reason: 'test', force: true })
      })

      const callCountAfterDelay = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      expect(callCountAfterDelay).toBe(callCountAfterOpen + 1)
    })

    it('guards against concurrent in-flight delta syncs', async () => {
      let resolveApi: ((value: ActivityDeltaResponse) => void) | null = null
      const api = vi.fn(
        () =>
          new Promise<ActivityDeltaResponse>((resolve) => {
            resolveApi = resolve
          }),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      // Start first sync (it will hang until resolved)
      act(() => {
        lastSocket().emitOpen()
      })

      // Advance past debounce
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Try second sync while first is in-flight
      await act(async () => {
        await result.current.syncActivityDelta({
          reason: 'test2',
          force: true,
        })
      })

      const deltaCallCount = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      // Only 1 call should have been made (the second was blocked by in-flight guard)
      expect(deltaCallCount).toBe(1)

      // Resolve the pending request
      await act(async () => {
        resolveApi?.(defaultDeltaResponse())
      })
    })

    it('skips delta sync when not connected and force is false', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      // Do NOT open socket, so eventsSocketConnected remains false
      // Call syncActivityDelta without force
      await act(async () => {
        await result.current.syncActivityDelta({ reason: 'test' })
      })

      const deltaCalls = (api as ReturnType<typeof vi.fn>).mock.calls.filter(
        (c) => String(c[0]).includes('/api/tmux/activity/delta'),
      )
      expect(deltaCalls).toHaveLength(0)
    })

    it('performs delta sync when disconnected but force is true', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        await result.current.syncActivityDelta({ reason: 'test', force: true })
      })

      const deltaCalls = (api as ReturnType<typeof vi.fn>).mock.calls.filter(
        (c) => String(c[0]).includes('/api/tmux/activity/delta'),
      )
      expect(deltaCalls).toHaveLength(1)
    })

    it('increments deltaSyncCount metric on each sync', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      expect(opts.runtimeMetricsRef.current.deltaSyncCount).toBe(1)
    })

    it('increments deltaSyncErrors on API failure', async () => {
      const api = vi.fn(() =>
        Promise.reject(new Error('network error')),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      expect(opts.runtimeMetricsRef.current.deltaSyncErrors).toBe(1)
    })

    it('triggers full refresh on overflow', async () => {
      const api = vi.fn(() =>
        Promise.resolve({
          ...defaultDeltaResponse(),
          overflow: true,
        }),
      ) as unknown as ApiFunction
      const refreshSessions = vi.fn(() => Promise.resolve())
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({
        api,
        refreshSessions,
        refreshInspector,
        tabsStateRef: makeRef({
          activeSession: 'main',
          sessions: [],
          activeWindowIndex: null,
          activePaneID: null,
        }) as TabsStateRef,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      expect(refreshSessions).toHaveBeenCalled()
      expect(refreshInspector).toHaveBeenCalledWith('main', {
        background: true,
      })
      expect(opts.runtimeMetricsRef.current.deltaOverflowCount).toBe(1)
    })

    it('updates globalRev from delta response', async () => {
      const api = vi.fn(() =>
        Promise.resolve({ ...defaultDeltaResponse(), globalRev: 42 }),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // First sync happened with since=0, response has globalRev=42
      const firstDeltaCall = (api as ReturnType<typeof vi.fn>).mock.calls
        .filter((c) => String(c[0]).includes('/api/tmux/activity/delta'))
        .pop()
      expect(firstDeltaCall).toBeDefined()
      expect(String(firstDeltaCall![0])).toContain('since=0')

      // Advance past debounce window (900ms)
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Trigger another sync directly via the returned function
      ;(api as ReturnType<typeof vi.fn>).mockResolvedValue(
        defaultDeltaResponse(),
      )
      await act(async () => {
        await result.current.syncActivityDelta({ reason: 'test', force: true })
      })

      // The second call should use since=42 (from the first response)
      const secondDeltaCall = (api as ReturnType<typeof vi.fn>).mock.calls
        .filter((c) => String(c[0]).includes('/api/tmux/activity/delta'))
        .pop()
      expect(secondDeltaCall).toBeDefined()
      expect(String(secondDeltaCall![0])).toContain('since=42')
    })
  })

  // -------------------------------------------------------------------------
  // 4. Adaptive fallback polling when WS disconnects
  // -------------------------------------------------------------------------

  describe('adaptive fallback polling', () => {
    it('starts polling when WS is not connected', () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      // The initial hook render triggers refreshAllState before WS connects
      expect(refreshSessions).toHaveBeenCalled()

      // Advance past the 8s polling interval
      const callCountBefore = refreshSessions.mock.calls.length
      act(() => {
        vi.advanceTimersByTime(8_100)
      })

      expect(refreshSessions.mock.calls.length).toBeGreaterThan(callCountBefore)
    })

    it('increments fallbackRefreshCount during polling', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      // The initial render increments it
      const countBefore = opts.runtimeMetricsRef.current.fallbackRefreshCount

      act(() => {
        vi.advanceTimersByTime(8_100)
      })

      expect(
        opts.runtimeMetricsRef.current.fallbackRefreshCount,
      ).toBeGreaterThan(countBefore)
    })

    it('stops fallback polling when WS connects', () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      // Connect the WebSocket
      act(() => {
        lastSocket().emitOpen()
      })

      const callCount = refreshSessions.mock.calls.length

      // Advance well past the 8s polling interval
      act(() => {
        vi.advanceTimersByTime(20_000)
      })

      // Should NOT have more calls from fallback polling (only scheduled refreshes)
      // The count might increase from scheduled refreshes, but not from the
      // 8s interval
      expect(refreshSessions.mock.calls.length).toBeLessThanOrEqual(
        callCount + 1,
      )
    })

    it('does not poll when tokenRequired and not authenticated', () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({
        authenticated: false,
        tokenRequired: true,
        refreshSessions,
      })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        vi.advanceTimersByTime(20_000)
      })

      // Should not have any refreshes when not authenticated
      expect(refreshSessions).not.toHaveBeenCalled()
    })

    it('resumes polling after WS disconnects', () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      // Connect then disconnect
      act(() => {
        lastSocket().emitOpen()
      })

      const callCountConnected = refreshSessions.mock.calls.length

      act(() => {
        lastSocket().emitClose()
      })

      const callCountAfterDisconnect = refreshSessions.mock.calls.length

      // Fallback polling should resume after disconnect
      act(() => {
        vi.advanceTimersByTime(8_100)
      })

      expect(refreshSessions.mock.calls.length).toBeGreaterThan(
        callCountAfterDisconnect,
      )
    })
  })

  // -------------------------------------------------------------------------
  // 5. Exponential backoff with jitter on reconnect
  // -------------------------------------------------------------------------

  describe('exponential backoff on reconnect', () => {
    it('reconnects after close with exponential backoff', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      const initialCount = MockWebSocket.instances.length

      act(() => {
        lastSocket().emitOpen()
      })

      // Close triggers reconnect attempt
      act(() => {
        lastSocket().emitClose()
      })

      // Advance timer past max possible delay for attempt 1:
      // baseDelay = min(10000, 500 * 2^1) = 1000, jitter = 0..299
      // so max is 1299
      act(() => {
        vi.advanceTimersByTime(1_300)
      })

      expect(MockWebSocket.instances.length).toBeGreaterThan(initialCount)
    })

    it('increases delay on successive reconnect failures', () => {
      const timeoutSpy = vi.spyOn(window, 'setTimeout')
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Track delays on successive closes
      const delays: Array<number> = []

      // Close 1 (attempt=1): base = min(10000, 500*2^1) = 1000
      act(() => {
        lastSocket().emitClose()
      })
      const call1 = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      delays.push(call1[1] as number)

      act(() => {
        vi.advanceTimersByTime(15_000)
      })

      // Close 2 (attempt=2): base = min(10000, 500*2^2) = 2000
      act(() => {
        lastSocket().emitClose()
      })
      const call2 = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      delays.push(call2[1] as number)

      act(() => {
        vi.advanceTimersByTime(15_000)
      })

      // Close 3 (attempt=3): base = min(10000, 500*2^3) = 4000
      act(() => {
        lastSocket().emitClose()
      })
      const call3 = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      delays.push(call3[1] as number)

      // Each delay should be larger than the previous (ignoring jitter variation)
      // The base delays are 1000, 2000, 4000 — jitter is 0..299
      expect(delays[0]).toBeGreaterThanOrEqual(1000)
      expect(delays[0]).toBeLessThanOrEqual(1299)
      expect(delays[1]).toBeGreaterThanOrEqual(2000)
      expect(delays[1]).toBeLessThanOrEqual(2299)
      expect(delays[2]).toBeGreaterThanOrEqual(4000)
      expect(delays[2]).toBeLessThanOrEqual(4299)
    })

    it('caps backoff delay at 10 seconds', () => {
      const timeoutSpy = vi.spyOn(window, 'setTimeout')
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Simulate many reconnect failures to reach the cap
      // expo = min(8, attempt), base = min(10000, 500 * 2^expo)
      // At attempt=8+: expo=8, base = min(10000, 500*256) = 10000
      for (let i = 0; i < 12; i++) {
        act(() => {
          lastSocket().emitClose()
          vi.advanceTimersByTime(15_000)
        })
      }

      // Check the last delay
      const lastCall = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      const lastDelay = lastCall[1] as number

      // Should be capped: base 10000 + jitter 0..299 => max 10299
      expect(lastDelay).toBeGreaterThanOrEqual(10_000)
      expect(lastDelay).toBeLessThanOrEqual(10_299)
    })

    it('resets reconnect attempts on successful open', () => {
      const timeoutSpy = vi.spyOn(window, 'setTimeout')
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // First close (attempt goes to 1)
      act(() => {
        lastSocket().emitClose()
        vi.advanceTimersByTime(1_300)
      })

      // Second close (attempt goes to 2)
      act(() => {
        lastSocket().emitClose()
        vi.advanceTimersByTime(2_500)
      })

      // Successful open resets attempt counter
      act(() => {
        lastSocket().emitOpen()
      })

      // Close again — should use attempt=1 delay
      act(() => {
        lastSocket().emitClose()
      })

      const lastCall = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      const delay = lastCall[1] as number

      // attempt=1: base = min(10000, 500*2^1) = 1000, jitter 0..299
      expect(delay).toBeGreaterThanOrEqual(1000)
      expect(delay).toBeLessThanOrEqual(1299)
    })

    it('increments wsReconnects metric on each reconnect', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
        lastSocket().emitClose()
      })

      expect(opts.runtimeMetricsRef.current.wsReconnects).toBe(1)

      act(() => {
        vi.advanceTimersByTime(1_300)
      })

      act(() => {
        lastSocket().emitClose()
      })

      expect(opts.runtimeMetricsRef.current.wsReconnects).toBe(2)
    })

    it('does not reconnect after unmount', () => {
      const opts = makeOptions()
      const { unmount } = renderHook(() => useTmuxEventsSocket(opts))

      const socket = lastSocket()
      act(() => {
        socket.emitOpen()
      })

      const instancesBefore = MockWebSocket.instances.length
      unmount()

      // The unmount closes socket and sets closed=true, so onclose
      // should not schedule reconnect
      act(() => {
        vi.advanceTimersByTime(15_000)
      })

      expect(MockWebSocket.instances.length).toBe(instancesBefore)
    })
  })

  // -------------------------------------------------------------------------
  // 6. Event gap handling
  // -------------------------------------------------------------------------

  describe('event gap handling', () => {
    it('triggers delta sync on event ID gap for activity events', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // First message with eventId 1
      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.activity.updated',
          eventId: 1,
          payload: {},
        })
      })

      // Advance past debounce
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Message with eventId 5 (gap of 3)
      await act(async () => {
        lastSocket().emitMessage({
          type: 'tmux.activity.updated',
          eventId: 5,
          payload: {},
        })
        await vi.advanceTimersByTimeAsync(100)
      })

      // The gap should trigger a delta sync
      const deltaCalls = (api as ReturnType<typeof vi.fn>).mock.calls.filter(
        (c) => String(c[0]).includes('/api/tmux/activity/delta'),
      )
      expect(deltaCalls.length).toBeGreaterThanOrEqual(2) // initial + gap
    })

    it('triggers delta sync on event ID gap for session events', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // First message
      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'created' },
        })
      })

      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Message with gap
      await act(async () => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 10,
          payload: { action: 'created' },
        })
        await vi.advanceTimersByTimeAsync(100)
      })

      const deltaCalls = (api as ReturnType<typeof vi.fn>).mock.calls.filter(
        (c) => String(c[0]).includes('/api/tmux/activity/delta'),
      )
      expect(deltaCalls.length).toBeGreaterThanOrEqual(2)
    })

    it('schedules refresh recovery on event ID gap for inspector events', async () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const refreshInspector = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions, refreshInspector })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 1,
          payload: { action: 'select-pane', session: 'main' },
        })
      })

      await act(async () => {
        lastSocket().emitMessage({
          type: 'tmux.inspector.updated',
          eventId: 4,
          payload: { action: 'select-pane', session: 'main' },
        })
        await vi.advanceTimersByTimeAsync(250)
      })

      expect(refreshSessions).toHaveBeenCalled()
      expect(refreshInspector).toHaveBeenCalledWith('main', {
        background: true,
      })
    })
  })

  // -------------------------------------------------------------------------
  // 7. Session refresh scheduling
  // -------------------------------------------------------------------------

  describe('session refresh scheduling', () => {
    it('schedules sessions refresh when shouldRefreshSessionsFromEvent returns true', () => {
      mockedShouldRefreshSessions.mockReturnValue({ refresh: true })
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const callsBefore = refreshSessions.mock.calls.length

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'created' },
        })
      })

      // The refresh is scheduled with a 180ms delay
      act(() => {
        vi.advanceTimersByTime(200)
      })

      expect(refreshSessions.mock.calls.length).toBeGreaterThan(callsBefore)
    })

    it('does not schedule sessions refresh when shouldRefreshSessionsFromEvent returns false', () => {
      mockedShouldRefreshSessions.mockReturnValue({ refresh: false })
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const callsAfterOpen = refreshSessions.mock.calls.length

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'activity' },
        })
      })

      act(() => {
        vi.advanceTimersByTime(500)
      })

      // No new sessions refresh calls from the event
      expect(refreshSessions.mock.calls.length).toBe(callsAfterOpen)
    })

    it('coalesces multiple scheduled refreshes of the same kind', () => {
      mockedShouldRefreshSessions.mockReturnValue({ refresh: true })
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const callsAfterOpen = refreshSessions.mock.calls.length

      // Send two events rapidly
      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'created' },
        })
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 2,
          payload: { action: 'created' },
        })
      })

      // Only one refresh should fire (the second schedule is a no-op
      // because a timer is already pending)
      act(() => {
        vi.advanceTimersByTime(200)
      })

      expect(refreshSessions.mock.calls.length).toBe(callsAfterOpen + 1)
    })
  })

  // -------------------------------------------------------------------------
  // 8. Visibility and online reconciliation
  // -------------------------------------------------------------------------

  describe('visibility and online reconciliation', () => {
    it('triggers delta sync when page becomes visible while connected', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      const deltaCountBefore = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      // Advance past debounce
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Simulate visibility change
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })

      await act(async () => {
        document.dispatchEvent(new Event('visibilitychange'))
      })

      const deltaCountAfter = (
        api as ReturnType<typeof vi.fn>
      ).mock.calls.filter((c) =>
        String(c[0]).includes('/api/tmux/activity/delta'),
      ).length

      expect(deltaCountAfter).toBeGreaterThan(deltaCountBefore)
    })

    it('triggers refreshAllState on online event when WS is disconnected', () => {
      const refreshSessions = vi.fn(() => Promise.resolve())
      const opts = makeOptions({ refreshSessions })
      renderHook(() => useTmuxEventsSocket(opts))

      const callsBefore = refreshSessions.mock.calls.length

      act(() => {
        window.dispatchEvent(new Event('online'))
      })

      expect(refreshSessions.mock.calls.length).toBeGreaterThan(callsBefore)
    })
  })

  // -------------------------------------------------------------------------
  // 9. Global revision tracking from messages
  // -------------------------------------------------------------------------

  describe('global revision tracking', () => {
    it('tracks globalRev from message-level field', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      // Send a message with globalRev at message level
      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          globalRev: 50,
          payload: { action: 'created' },
        })
      })

      // Advance past debounce
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      // Next delta sync should use since=50
      ;(api as ReturnType<typeof vi.fn>).mockResolvedValue(
        defaultDeltaResponse(),
      )

      // Force a gap to trigger sync
      await act(async () => {
        lastSocket().emitMessage({
          type: 'tmux.activity.updated',
          eventId: 100,
          payload: {},
        })
        await vi.advanceTimersByTimeAsync(100)
      })

      const lastDeltaCall = (api as ReturnType<typeof vi.fn>).mock.calls
        .filter((c) => String(c[0]).includes('/api/tmux/activity/delta'))
        .pop()
      if (lastDeltaCall) {
        expect(String(lastDeltaCall[0])).toContain('since=50')
      }
    })

    it('tracks globalRev from payload-level field', async () => {
      const api = vi.fn(() =>
        Promise.resolve(defaultDeltaResponse()),
      ) as unknown as ApiFunction
      const opts = makeOptions({ api })
      renderHook(() => useTmuxEventsSocket(opts))

      await act(async () => {
        lastSocket().emitOpen()
      })

      act(() => {
        lastSocket().emitMessage({
          type: 'tmux.sessions.updated',
          eventId: 1,
          payload: { action: 'created', globalRev: 75 },
        })
      })

      act(() => {
        vi.advanceTimersByTime(1000)
      })
      ;(api as ReturnType<typeof vi.fn>).mockResolvedValue(
        defaultDeltaResponse(),
      )

      await act(async () => {
        lastSocket().emitMessage({
          type: 'tmux.activity.updated',
          eventId: 100,
          payload: {},
        })
        await vi.advanceTimersByTimeAsync(100)
      })

      const lastDeltaCall = (api as ReturnType<typeof vi.fn>).mock.calls
        .filter((c) => String(c[0]).includes('/api/tmux/activity/delta'))
        .pop()
      if (lastDeltaCall) {
        expect(String(lastDeltaCall[0])).toContain('since=75')
      }
    })
  })

  // -------------------------------------------------------------------------
  // 10. Force reconnect
  // -------------------------------------------------------------------------

  describe('forceReconnect', () => {
    it('returns forceReconnect function from the hook', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      expect(typeof result.current.forceReconnect).toBe('function')
    })

    it('closes current socket and opens a new one', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      const firstSocket = lastSocket()
      act(() => {
        firstSocket.emitOpen()
      })
      expect(result.current.eventsSocketConnected).toBe(true)

      const instancesBefore = MockWebSocket.instances.length

      act(() => {
        result.current.forceReconnect()
      })

      expect(firstSocket.closed).toBe(true)
      expect(MockWebSocket.instances.length).toBeGreaterThan(instancesBefore)
    })

    it('resets reconnect attempt counter', () => {
      const timeoutSpy = vi.spyOn(window, 'setTimeout')
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Build up reconnect attempts via successive closes
      act(() => {
        lastSocket().emitClose()
        vi.advanceTimersByTime(1_300)
      })
      act(() => {
        lastSocket().emitClose()
        vi.advanceTimersByTime(2_500)
      })
      act(() => {
        lastSocket().emitClose()
        vi.advanceTimersByTime(4_500)
      })

      // Force reconnect should reset the counter
      act(() => {
        result.current.forceReconnect()
      })

      // The new socket opens, then closes — delay should be attempt=1 level
      act(() => {
        lastSocket().emitOpen()
        lastSocket().emitClose()
      })

      const lastCall = timeoutSpy.mock.calls[timeoutSpy.mock.calls.length - 1]
      const delay = lastCall[1] as number

      // attempt=1: base = min(10000, 500*2^1) = 1000, jitter 0..299
      expect(delay).toBeGreaterThanOrEqual(1000)
      expect(delay).toBeLessThanOrEqual(1299)
    })

    it('cancels pending retry timer before reconnecting', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      // Trigger a close which schedules a retry timer
      act(() => {
        lastSocket().emitClose()
      })

      const instancesBeforeForce = MockWebSocket.instances.length

      // Force reconnect before the retry timer fires
      act(() => {
        result.current.forceReconnect()
      })

      // The force reconnect should have created exactly one new socket
      // (not two — the pending retry should have been cancelled)
      const instancesAfterForce = MockWebSocket.instances.length
      expect(instancesAfterForce).toBe(instancesBeforeForce + 1)

      // Advance past what would have been the retry timer
      act(() => {
        vi.advanceTimersByTime(15_000)
      })

      // No additional sockets should have been created from the cancelled timer
      expect(MockWebSocket.instances.length).toBe(instancesAfterForce)
    })

    it('works when called with no active socket', () => {
      const opts = makeOptions({
        authenticated: false,
        tokenRequired: true,
      })
      const { result, rerender } = renderHook(
        (props: { authenticated: boolean }) =>
          useTmuxEventsSocket({
            ...opts,
            authenticated: props.authenticated,
          }),
        { initialProps: { authenticated: false } },
      )

      // No socket exists
      expect(MockWebSocket.instances).toHaveLength(0)

      // Enable authentication so that the effect will connect
      rerender({ authenticated: true })

      const instancesBefore = MockWebSocket.instances.length
      expect(instancesBefore).toBeGreaterThanOrEqual(1)

      // Force reconnect
      act(() => {
        result.current.forceReconnect()
      })

      expect(MockWebSocket.instances.length).toBeGreaterThan(instancesBefore)
    })

    it('prevents onclose handler from scheduling a retry after force close', () => {
      const opts = makeOptions()
      const { result } = renderHook(() => useTmuxEventsSocket(opts))

      act(() => {
        lastSocket().emitOpen()
      })

      const socketBeforeForce = lastSocket()

      act(() => {
        result.current.forceReconnect()
      })

      const instancesAfterForce = MockWebSocket.instances.length

      // Manually fire onclose on the old socket — it should be nulled out
      // by forceReconnect so this should not trigger another reconnect
      expect(socketBeforeForce.onclose).toBeNull()

      // No extra sockets created
      act(() => {
        vi.advanceTimersByTime(15_000)
      })
      expect(MockWebSocket.instances.length).toBe(instancesAfterForce)
    })
  })

  // -------------------------------------------------------------------------
  // 11. Runtime metrics exposure
  // -------------------------------------------------------------------------

  describe('runtime metrics', () => {
    it('exposes metrics on window.__SENTINEL_TMUX_METRICS', () => {
      const opts = makeOptions()
      renderHook(() => useTmuxEventsSocket(opts))

      const windowWithMetrics = window as typeof window & {
        __SENTINEL_TMUX_METRICS?: unknown
      }
      expect(windowWithMetrics.__SENTINEL_TMUX_METRICS).toBe(
        opts.runtimeMetricsRef.current,
      )
    })

    it('cleans up window.__SENTINEL_TMUX_METRICS on unmount', () => {
      const opts = makeOptions()
      const { unmount } = renderHook(() => useTmuxEventsSocket(opts))

      unmount()

      const windowWithMetrics = window as typeof window & {
        __SENTINEL_TMUX_METRICS?: unknown
      }
      expect(windowWithMetrics.__SENTINEL_TMUX_METRICS).toBeUndefined()
    })
  })
})
