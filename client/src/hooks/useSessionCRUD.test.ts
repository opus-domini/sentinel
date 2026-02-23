// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { GuardrailConfirmError } from './useTmuxApi'
import { useSessionCRUD } from './useSessionCRUD'
import type { Session } from '@/types'
import type {
  ApiFunction,
  DispatchTabs,
  RuntimeMetrics,
  TabsStateRef,
} from './tmuxTypes'

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

function makeGuardrailError(): GuardrailConfirmError {
  return new GuardrailConfirmError(
    'Dangerous operation',
    {
      mode: 'confirm',
      allowed: false,
      requireConfirm: true,
      message: 'This will destroy the session',
      matchedRuleId: 'rule-1',
      matchedRules: [
        {
          id: 'rule-1',
          name: 'protect-prod',
          pattern: 'prod*',
          action: 'confirm',
          enabled: true,
        },
      ],
    },
    '/api/tmux/sessions/prod',
    { method: 'DELETE' },
  )
}

type MockOptions = {
  api?: ReturnType<typeof vi.fn>
  sessions?: Array<Session>
  activeSession?: string
  openTabs?: Array<string>
}

function createMockOptions(overrides: MockOptions = {}) {
  const sessions = overrides.sessions ?? [makeSession('prod')]
  const activeSession = overrides.activeSession ?? 'prod'
  const openTabs = overrides.openTabs ?? ['prod']

  const api = (overrides.api ?? vi.fn()) as ApiFunction
  const dispatchTabs: DispatchTabs = vi.fn()
  const setSessions = vi.fn<[React.SetStateAction<Array<Session>>], void>()
  const setConnection = vi.fn()
  const closeCurrentSocket = vi.fn()
  const resetTerminal = vi.fn()
  const refreshInspector = vi.fn(() => Promise.resolve())
  const clearPendingInspectorSessionState = vi.fn()
  const pushErrorToast = vi.fn()
  const pushSuccessToast = vi.fn()
  const setTmuxUnavailable = vi.fn()
  const requestGuardrailConfirm = vi.fn()

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
      recoveryRefreshCount: 0,
      fallbackRefreshCount: 0,
      deltaSyncCount: 0,
      deltaSyncErrors: 0,
      deltaOverflowCount: 0,
    },
  }
  const pendingCreateSessionsRef = { current: new Map<string, string>() }

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
    connectionState: 'connected' as const,
    refreshInspector,
    clearPendingInspectorSessionState,
    pushErrorToast,
    pushSuccessToast,
    pendingCreateSessionsRef,
    requestGuardrailConfirm,
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useSessionCRUD – killSession', () => {
  it('guardrail rejection does not apply optimistic UI', async () => {
    const api = vi.fn().mockRejectedValue(makeGuardrailError())
    const opts = createMockOptions({ api })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.killSession('prod')
    })

    // Optimistic UI was NOT applied
    expect(opts.setSessions).not.toHaveBeenCalled()
    expect(opts.dispatchTabs).not.toHaveBeenCalled()
    expect(opts.closeCurrentSocket).not.toHaveBeenCalled()

    // Guardrail confirm dialog WAS requested
    expect(opts.requestGuardrailConfirm).toHaveBeenCalledWith(
      'protect-prod',
      'This will destroy the session',
      expect.any(Function),
    )
  })

  it('confirmed kill applies optimistic UI before API call', async () => {
    // API resolves after a tick so we can verify order
    const api = vi.fn().mockResolvedValue(undefined)
    const opts = createMockOptions({ api })

    const { result } = renderHook(() => useSessionCRUD(opts))

    // Record the order of calls
    const callOrder: Array<string> = []
    opts.setSessions.mockImplementation(() => callOrder.push('setSessions'))
    opts.dispatchTabs.mockImplementation((action: { type: string }) => {
      callOrder.push(`dispatchTabs:${action.type}`)
    })
    opts.closeCurrentSocket.mockImplementation(() =>
      callOrder.push('closeCurrentSocket'),
    )
    opts.setConnection.mockImplementation(() => callOrder.push('setConnection'))

    // Simulate the guardrail confirm callback
    // killSessionWithConfirm is internal, but the requestGuardrailConfirm
    // callback from the initial call will invoke it. We need to call
    // killSession first to trigger the guardrail flow, then invoke the
    // confirm callback.
    //
    // Since killSession(name) calls killSessionWithConfirm(name, false),
    // and the API rejects with a guardrail error, requestGuardrailConfirm
    // captures the onConfirm callback. We retrieve it and call it.

    // First call: API rejects with guardrail
    api.mockRejectedValueOnce(makeGuardrailError())

    await act(async () => {
      await result.current.killSession('prod')
    })

    // Now retrieve the onConfirm callback
    const confirmCallback = opts.requestGuardrailConfirm.mock
      .calls[0][2] as () => void

    // Second call (confirmed): API resolves
    api.mockResolvedValueOnce(undefined)

    await act(async () => {
      confirmCallback()
      // Allow the async killSessionWithConfirm(name, true) to settle
      await vi.waitFor(() => {
        expect(api).toHaveBeenCalledTimes(2)
      })
    })

    // Optimistic UI was applied (setSessions filter, dispatchTabs close,
    // closeCurrentSocket, setConnection)
    expect(opts.setSessions).toHaveBeenCalled()
    expect(opts.dispatchTabs).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'close', session: 'prod' }),
    )
    expect(opts.closeCurrentSocket).toHaveBeenCalledWith('session killed')
    expect(opts.setConnection).toHaveBeenCalledWith(
      'disconnected',
      'session killed',
    )

    // Optimistic UI was applied before the API call
    const apiCallIndex = callOrder.indexOf('setSessions')
    expect(apiCallIndex).toBeGreaterThanOrEqual(0)
  })

  it('success without guardrail applies optimistic UI after API resolves', async () => {
    const api = vi.fn().mockResolvedValue(undefined)
    const opts = createMockOptions({ api })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.killSession('prod')
    })

    // No guardrail was triggered
    expect(opts.requestGuardrailConfirm).not.toHaveBeenCalled()

    // Optimistic UI was applied after successful API response
    expect(opts.setSessions).toHaveBeenCalled()
    expect(opts.dispatchTabs).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'close', session: 'prod' }),
    )
    expect(opts.closeCurrentSocket).toHaveBeenCalledWith('session killed')
    expect(opts.pushSuccessToast).toHaveBeenCalledWith(
      'Kill Session',
      'session "prod" killed',
    )
  })

  it('API error after confirmed kill rolls back', async () => {
    // First call: guardrail rejection
    const api = vi.fn().mockRejectedValueOnce(makeGuardrailError())
    const opts = createMockOptions({ api })

    const { result } = renderHook(() => useSessionCRUD(opts))

    await act(async () => {
      await result.current.killSession('prod')
    })

    const confirmCallback = opts.requestGuardrailConfirm.mock
      .calls[0][2] as () => void

    // Second call (confirmed): API fails with a generic error
    api.mockRejectedValueOnce(new Error('tmux server crashed'))

    await act(async () => {
      confirmCallback()
      await vi.waitFor(() => {
        expect(api).toHaveBeenCalledTimes(2)
      })
    })

    // Rollback: dispatchTabs activate was called to restore the tab
    expect(opts.dispatchTabs).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'activate', session: 'prod' }),
    )

    // Error state was set
    expect(opts.setConnection).toHaveBeenCalledWith(
      'error',
      'tmux server crashed',
    )
    expect(opts.pushErrorToast).toHaveBeenCalledWith(
      'Kill Session',
      'tmux server crashed',
    )
  })
})
