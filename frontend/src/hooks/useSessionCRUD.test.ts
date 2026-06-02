// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useSessionCRUD } from './useSessionCRUD'
import type { Session } from '@/types'
import type { Dispatch, SetStateAction } from 'react'
import type { ApiFunction, DispatchTabs, RuntimeMetrics, TabsStateRef } from './tmuxTypes'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(name: string, overrides?: Partial<Session>): Session {
  return {
    name,
    windows: 1,
    panes: 1,
    attached: 1,
    createdAt: new Date().toISOString(),
    activityAt: new Date().toISOString(),
    command: 'bash',
    hash: 'abc',
    lastContent: '',
    icon: '',
    ...overrides,
  }
}

type MockOptions = {
  api?: ReturnType<typeof vi.fn>
  sessions?: Array<Session>
  activeSession?: string
  openTabs?: Array<string>
  connectionState?: 'connected' | 'connecting' | 'disconnected' | 'error'
}

function createMockOptions(overrides: MockOptions = {}) {
  const sessions = overrides.sessions ?? [makeSession('prod')]
  const activeSession = overrides.activeSession ?? 'prod'
  const openTabs = overrides.openTabs ?? ['prod']
  const connectionState = overrides.connectionState ?? 'connected'

  const api = (overrides.api ?? vi.fn()) as ApiFunction
  const dispatchTabs = vi.fn<DispatchTabs>()
  const setSessions = vi.fn<Dispatch<SetStateAction<Array<Session>>>>()
  const setConnection = vi.fn()
  const closeCurrentSocket = vi.fn()
  const resetTerminal = vi.fn()
  const refreshInspector = vi.fn(() => Promise.resolve())
  const clearPendingInspectorSessionState = vi.fn()
  const pushErrorToast = vi.fn()
  const pushSuccessToast = vi.fn()
  const setTmuxUnavailable = vi.fn()
  const refreshSessionPresets = vi.fn(() => Promise.resolve())

  const tabsStateRef: TabsStateRef = {
    current: { openTabs, activeSession, activeEpoch: 0 },
  }
  const sessionsRef = { current: sessions }
  const runtimeMetricsRef: { current: RuntimeMetrics } = {
    current: {
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
    },
  }
  const pendingCreateSessionsRef = { current: new Map<string, string>() }
  const pendingKillSessionsRef = { current: new Set<string>() }

  return {
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
    pendingKillSessionsRef,
    refreshSessionPresets,
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useSessionCRUD – killSession', () => {
  it('applies optimistic UI after API resolves', async () => {
    const api = vi.fn().mockResolvedValue(undefined)
    const opts = createMockOptions({ api })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.killSession('prod')
    })

    // Optimistic UI was applied after successful API response
    expect(opts.setSessions).toHaveBeenCalled()
    expect(opts.dispatchTabs).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'close', session: 'prod' }),
    )
    expect(opts.closeCurrentSocket).toHaveBeenCalledWith('session killed')
    expect(opts.pushSuccessToast).toHaveBeenCalledWith('Kill Session', 'session "prod" killed')
  })
})

describe('useSessionCRUD – createSession', () => {
  it('sends icon when creating a session', async () => {
    const api = vi.fn().mockResolvedValueOnce({ name: 'dev' })
    const opts = createMockOptions({
      api,
      sessions: [],
      activeSession: '',
      openTabs: [],
    })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.createSession('dev', '/tmp', 'code')
    })

    expect(api).toHaveBeenNthCalledWith(1, '/api/tmux/sessions', {
      method: 'POST',
      body: expect.any(String),
    })
    const request = api.mock.calls[0]?.[1]
    const body = typeof request?.body === 'string' ? JSON.parse(request.body) : null
    expect(body).toMatchObject({
      name: 'dev',
      cwd: '/tmp',
      icon: 'code',
      operationId: expect.stringMatching(/^session-create-/),
    })
    expect(
      opts.dispatchTabs.mock.calls.filter(
        ([action]) => action.type === 'activate' && action.session === 'dev',
      ),
    ).toHaveLength(2)
  })

  it('keeps pending create until the sessions list confirms the new session', async () => {
    const api = vi
      .fn()
      .mockResolvedValueOnce({ name: 'dev' })
      .mockResolvedValueOnce({ sessions: [] })
    const opts = createMockOptions({
      api,
      sessions: [],
      activeSession: '',
      openTabs: [],
    })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.createSession('dev', '/tmp')
    })

    expect(opts.refreshInspector).not.toHaveBeenCalled()
    expect(opts.pendingCreateSessionsRef.current.has('dev')).toBe(true)

    const request = api.mock.calls[0]?.[1]
    const body = typeof request?.body === 'string' ? JSON.parse(request.body) : null

    act(() => {
      result.current.handleTmuxSessionsEvent({
        action: 'create',
        session: 'dev',
        operationId: body?.operationId,
      })
    })

    expect(opts.refreshInspector).toHaveBeenCalledWith('dev', { force: true })
    expect(opts.pendingCreateSessionsRef.current.has('dev')).toBe(true)
  })

  it('clears pending create after refreshSessions sees the session in backend state', async () => {
    const api = vi.fn().mockResolvedValue({
      sessions: [makeSession('dev')],
    })
    const opts = createMockOptions({
      api,
      sessions: [],
      activeSession: '',
      openTabs: [],
    })
    opts.pendingCreateSessionsRef.current.set('dev', '2026-02-14T12:00:00Z')

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(opts.pendingCreateSessionsRef.current.has('dev')).toBe(false)
  })

  it('does not roll back a created session that already attached before list convergence', async () => {
    vi.useFakeTimers()
    try {
      const api = vi
        .fn()
        .mockResolvedValueOnce({ name: 'dev' })
        .mockResolvedValueOnce({ sessions: [] })
      const opts = createMockOptions({
        api,
        sessions: [],
        activeSession: '',
        openTabs: [],
        connectionState: 'connecting',
      })

      const { result, rerender } = renderHook(() => useSessionCRUD(opts))

      await act(async () => {
        await result.current.createSession('dev', '/tmp')
      })

      act(() => {
        opts.tabsStateRef.current = { openTabs: ['dev'], activeSession: 'dev', activeEpoch: 1 }
        opts.connectionState = 'connected'
        rerender()
      })

      await act(async () => {
        await vi.advanceTimersByTimeAsync(4_000)
      })

      expect(opts.pushSuccessToast).toHaveBeenCalledWith('Create Session', 'session "dev" created')
      expect(opts.pushErrorToast).not.toHaveBeenCalledWith(
        'Create Session',
        'timed out waiting for session "dev" to be ready',
      )
      expect(opts.dispatchTabs).not.toHaveBeenCalledWith({ type: 'close', session: 'dev' })
      expect(opts.setConnection).not.toHaveBeenCalledWith(
        'error',
        'timed out waiting for session "dev" to be ready',
      )
      expect(opts.pendingCreateSessionsRef.current.has('dev')).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('useSessionCRUD – refreshSessions', () => {
  it('preserves the active session while the terminal is still connecting', async () => {
    const api = vi.fn().mockResolvedValue({
      sessions: [makeSession('Home')],
    })
    const opts = createMockOptions({
      api,
      sessions: [makeSession('Hugo'), makeSession('Home')],
      activeSession: 'Hugo',
      openTabs: ['Hugo'],
      connectionState: 'connecting',
    })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(opts.closeCurrentSocket).not.toHaveBeenCalled()
    expect(opts.resetTerminal).not.toHaveBeenCalled()
    expect(opts.setConnection).not.toHaveBeenCalledWith('disconnected', 'active session removed')
    expect(opts.dispatchTabs).toHaveBeenCalledWith({
      type: 'sync',
      sessions: ['Hugo', 'Home'],
    })
    expect(opts.setSessions).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ name: 'Hugo' }),
        expect.objectContaining({ name: 'Home' }),
      ]),
    )
  })

  it('uses the latest connection state when preserving a connecting active session', async () => {
    const api = vi.fn().mockResolvedValue({
      sessions: [makeSession('Home')],
    })
    const opts = createMockOptions({
      api,
      sessions: [makeSession('Hugo'), makeSession('Home')],
      activeSession: 'Hugo',
      openTabs: ['Hugo'],
      connectionState: 'disconnected',
    })

    const { result, rerender } = renderHook(() => useSessionCRUD(opts))

    opts.connectionState = 'connecting'
    rerender()

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(opts.closeCurrentSocket).not.toHaveBeenCalled()
    expect(opts.resetTerminal).not.toHaveBeenCalled()
    expect(opts.dispatchTabs).toHaveBeenCalledWith({
      type: 'sync',
      sessions: ['Hugo', 'Home'],
    })
  })

  it('removes the active session when it disappears after the terminal is connected', async () => {
    const api = vi.fn().mockResolvedValue({
      sessions: [makeSession('Home')],
    })
    const opts = createMockOptions({
      api,
      sessions: [makeSession('Hugo'), makeSession('Home')],
      activeSession: 'Hugo',
      openTabs: ['Hugo'],
      connectionState: 'connected',
    })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(opts.closeCurrentSocket).toHaveBeenCalledWith('active session removed')
    expect(opts.resetTerminal).toHaveBeenCalled()
    expect(opts.setConnection).toHaveBeenCalledWith('disconnected', 'active session removed')
    expect(opts.dispatchTabs).toHaveBeenCalledWith({
      type: 'sync',
      sessions: ['Home'],
    })
  })
})
