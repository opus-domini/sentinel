// @vitest-environment jsdom
import { createElement } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import { useInspector } from './useInspector'
import type { ReactNode } from 'react'
import { buildPendingSplitPaneID } from '@/lib/tmuxInspectorOptimistic'
import type { PaneInfo, Session, WindowInfo } from '@/types'
import type { ApiFunction, RuntimeMetrics, TabsStateRef } from './tmuxTypes'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeWindow(overrides?: Partial<WindowInfo>): WindowInfo {
  return {
    session: 'dev',
    index: 0,
    name: 'main',
    displayName: 'main',
    active: true,
    panes: 1,
    ...overrides,
  }
}

function makePane(overrides?: Partial<PaneInfo>): PaneInfo {
  return {
    session: 'dev',
    windowIndex: 0,
    paneIndex: 0,
    paneId: '%1',
    title: 'shell',
    active: true,
    tty: '/dev/pts/1',
    ...overrides,
  }
}

function makeSession(name: string): Session {
  return {
    name,
    windows: 2,
    panes: 2,
    attached: 1,
    createdAt: new Date().toISOString(),
    activityAt: new Date().toISOString(),
    command: 'bash',
    hash: 'abc',
    lastContent: '',
    icon: '',
  }
}

/**
 * Build an API mock that returns the given windows and panes for inspector
 * endpoints, and resolves successfully for select-window / select-pane.
 */
function makeApi(windows: Array<WindowInfo>, panes: Array<PaneInfo>) {
  return vi.fn((url: string) => {
    if (typeof url === 'string' && url.includes('/windows')) {
      return Promise.resolve({ windows })
    }
    if (typeof url === 'string' && url.includes('/panes')) {
      return Promise.resolve({ panes })
    }
    // select-window / select-pane — resolve void
    return Promise.resolve(undefined)
  }) as unknown as ApiFunction
}

type MockOptions = {
  api?: ApiFunction
  activeSession?: string
  windows?: Array<WindowInfo>
  panes?: Array<PaneInfo>
}

function createMockOptions(overrides: MockOptions = {}) {
  const windows = overrides.windows ?? [
    makeWindow({ index: 0, active: true }),
    makeWindow({ index: 1, name: 'alt', active: false }),
  ]
  const panes = overrides.panes ?? [
    makePane({ windowIndex: 0, paneId: '%1', active: true }),
    makePane({ windowIndex: 1, paneId: '%2', paneIndex: 0, active: false }),
  ]
  const api = overrides.api ?? makeApi(windows, panes)
  const activeSession = overrides.activeSession ?? 'dev'

  const tabsStateRef: TabsStateRef = {
    current: { openTabs: [activeSession], activeSession, activeEpoch: 0 },
  }
  const sessionsRef = { current: [makeSession(activeSession)] }
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

  return {
    api,
    tabsStateRef,
    sessionsRef,
    runtimeMetricsRef,
    activeSession,
    setTmuxUnavailable: vi.fn(),
    setSessions: vi.fn(),
    refreshSessions: vi.fn(() => Promise.resolve()),
    pushErrorToast: vi.fn(),
    pushSuccessToast: vi.fn(),
    setConnection: vi.fn(),
    requestGuardrailConfirm: vi.fn(),
  }
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return {
    queryClient,
    wrapper: ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children),
  }
}

// ---------------------------------------------------------------------------
// Tests — selectWindow optimistic override
// ---------------------------------------------------------------------------

describe('useInspector – selectWindow', () => {
  it('sets optimistic window override immediately', async () => {
    const opts = createMockOptions()
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    // Wait for initial refreshInspector to settle
    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    act(() => {
      result.current.selectWindow(1)
    })

    expect(result.current.activeWindowIndexOverride).toBe(1)
  })

  it('clears override on API error and shows error', async () => {
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        return Promise.resolve({
          windows: [
            makeWindow({ index: 0, active: true }),
            makeWindow({ index: 1, name: 'alt', active: false }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({ windowIndex: 0, paneId: '%1', active: true }),
            makePane({ windowIndex: 1, paneId: '%2', active: false }),
          ],
        })
      }
      return Promise.reject(new Error('tmux select-window failed'))
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    await act(async () => {
      result.current.selectWindow(1)
      // Allow the rejection to propagate
      await vi.waitFor(() => {
        expect(opts.pushErrorToast).toHaveBeenCalled()
      })
    })

    expect(result.current.activeWindowIndexOverride).toBeNull()
    expect(opts.pushErrorToast).toHaveBeenCalledWith(
      'Switch Window',
      'tmux select-window failed',
    )
  })
})

describe('useInspector – reorderWindows', () => {
  it('sends stable tmux window ids to the reorder endpoint', async () => {
    const api = vi.fn((url: string, init?: RequestInit) => {
      if (typeof url === 'string' && url.includes('/windows/order')) {
        expect(init?.method).toBe('PATCH')
        expect(init?.body).toBe(JSON.stringify({ windowIds: ['@2', '@1'] }))
        return Promise.resolve(undefined)
      }
      if (typeof url === 'string' && url.includes('/windows')) {
        return Promise.resolve({
          windows: [
            makeWindow({ index: 0, active: true, tmuxWindowId: '@1' }),
            makeWindow({
              index: 1,
              name: 'runner',
              displayName: 'runner',
              active: false,
              tmuxWindowId: '@2',
            }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({ windowIndex: 0, paneId: '%1', active: true }),
            makePane({ windowIndex: 1, paneId: '%2', active: false }),
          ],
        })
      }
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows).toHaveLength(2)
    })

    act(() => {
      result.current.reorderWindows('@2', '@1')
    })

    await waitFor(() => {
      expect(api).toHaveBeenCalledWith(
        '/api/tmux/sessions/dev/windows/order',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ windowIds: ['@2', '@1'] }),
        }),
      )
    })
  })

  it('shows an error when runtime ids are unavailable', async () => {
    const opts = createMockOptions()
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows).toHaveLength(2)
    })

    act(() => {
      result.current.reorderWindows('@2', '@1')
    })

    expect(opts.pushErrorToast).toHaveBeenCalledWith(
      'Reorder Windows',
      'window order is not ready yet; refresh and try again',
    )
  })
})

describe('useInspector – window presentation stability', () => {
  it('preserves managed launcher presentation across degraded refreshes', async () => {
    let refreshCount = 0
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        refreshCount += 1
        if (refreshCount === 1) {
          return Promise.resolve({
            windows: [
              makeWindow({
                index: 0,
                name: 'claude',
                displayName: 'Claude Code',
                displayIcon: 'bot',
                tmuxWindowId: '@1',
                managed: true,
                managedWindowId: 'managed-1',
                launcherId: 'launcher-1',
              }),
            ],
          })
        }
        return Promise.resolve({
          windows: [
            makeWindow({
              index: 0,
              name: 'claude',
              displayName: 'claude',
              tmuxWindowId: '@1',
              managed: false,
            }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [makePane({ windowIndex: 0, paneId: '%1', active: true })],
        })
      }
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows[0]?.displayName).toBe('Claude Code')
    })

    await act(async () => {
      await result.current.refreshInspector('dev', { background: true })
    })

    expect(result.current.windows[0]).toMatchObject({
      displayName: 'Claude Code',
      displayIcon: 'bot',
      managed: true,
      managedWindowId: 'managed-1',
      launcherId: 'launcher-1',
    })
  })

  it('does not leak launcher presentation to a different runtime window at the same index', async () => {
    let refreshCount = 0
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        refreshCount += 1
        if (refreshCount === 1) {
          return Promise.resolve({
            windows: [
              makeWindow({
                index: 0,
                name: 'claude',
                displayName: 'Claude Code',
                displayIcon: 'bot',
                tmuxWindowId: '@1',
                managed: true,
                managedWindowId: 'managed-1',
                launcherId: 'launcher-1',
              }),
            ],
          })
        }
        return Promise.resolve({
          windows: [
            makeWindow({
              index: 0,
              name: 'shell',
              displayName: 'shell',
              tmuxWindowId: '@2',
              managed: false,
            }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [makePane({ windowIndex: 0, paneId: '%1', active: true })],
        })
      }
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows[0]?.displayName).toBe('Claude Code')
    })

    await act(async () => {
      await result.current.refreshInspector('dev', { background: true })
    })

    expect(result.current.windows[0]).toMatchObject({
      name: 'shell',
      displayName: 'shell',
      tmuxWindowId: '@2',
      managed: false,
    })
    expect(result.current.windows[0]?.displayIcon).toBeUndefined()
    expect(result.current.windows[0]?.managedWindowId).toBeUndefined()
    expect(result.current.windows[0]?.launcherId).toBeUndefined()
  })

  it('keeps unmanaged windows in sync with tmux renames on the same runtime window', async () => {
    let refreshCount = 0
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        refreshCount += 1
        if (refreshCount === 1) {
          return Promise.resolve({
            windows: [
              makeWindow({
                index: 0,
                name: 'shell',
                displayName: 'shell',
                tmuxWindowId: '@1',
                managed: false,
              }),
            ],
          })
        }
        return Promise.resolve({
          windows: [
            makeWindow({
              index: 0,
              name: 'runner',
              displayName: 'runner',
              tmuxWindowId: '@1',
              managed: false,
            }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [makePane({ windowIndex: 0, paneId: '%1', active: true })],
        })
      }
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows[0]?.displayName).toBe('shell')
    })

    await act(async () => {
      await result.current.refreshInspector('dev', { background: true })
    })

    expect(result.current.windows[0]).toMatchObject({
      name: 'runner',
      displayName: 'runner',
      tmuxWindowId: '@1',
      managed: false,
    })
  })
})

// ---------------------------------------------------------------------------
// Tests — refreshInspector override reconciliation
// ---------------------------------------------------------------------------

describe('useInspector – refreshInspector override reconciliation', () => {
  it('clears stale override when no mutation is in-flight', async () => {
    const windows = [
      makeWindow({ index: 0, active: false }),
      makeWindow({ index: 1, name: 'alt', active: true }),
    ]
    const panes = [
      makePane({ windowIndex: 0, paneId: '%1', active: false }),
      makePane({ windowIndex: 1, paneId: '%2', active: true }),
    ]
    const api = makeApi(windows, panes)
    const opts = createMockOptions({ api, windows, panes })
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    // Wait for initial refresh
    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    // Manually set an override (simulating a previous selectWindow that
    // already completed its API call — selectInFlightRef = 0)
    act(() => {
      result.current.setActiveWindowIndexOverride(0)
    })
    expect(result.current.activeWindowIndexOverride).toBe(0)

    // Trigger refreshInspector — server says window 1 is active
    await act(async () => {
      await result.current.refreshInspector('dev')
    })

    // Override should be cleared because selectInFlightRef is 0
    expect(result.current.activeWindowIndexOverride).toBeNull()
  })

  it('preserves override when mutation is in-flight', async () => {
    // API that never resolves select-window (simulating in-flight), and
    // returns stale data for /windows (window 0 active) until resolved.
    let resolveSelectWindow: (() => void) | null = null
    let serverConfirmed = false
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        return Promise.resolve({
          windows: [
            makeWindow({ index: 0, active: !serverConfirmed }),
            makeWindow({
              index: 1,
              name: 'alt',
              active: serverConfirmed,
            }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({
              windowIndex: 0,
              paneId: '%1',
              active: !serverConfirmed,
            }),
            makePane({
              windowIndex: 1,
              paneId: '%2',
              active: serverConfirmed,
            }),
          ],
        })
      }
      // select-window: return a promise that we control
      return new Promise<void>((resolve) => {
        resolveSelectWindow = resolve
      })
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    // Start selectWindow — API call hangs (selectInFlightRef > 0)
    act(() => {
      result.current.selectWindow(1)
    })
    expect(result.current.activeWindowIndexOverride).toBe(1)

    // Trigger refreshInspector while mutation is in-flight — server says
    // window 0 is still active (hasn't caught up yet)
    await act(async () => {
      await result.current.refreshInspector('dev')
    })

    // Override should be PRESERVED because selectInFlightRef > 0
    expect(result.current.activeWindowIndexOverride).toBe(1)

    // Now resolve the select-window API call and mark server as confirmed
    await act(async () => {
      serverConfirmed = true
      resolveSelectWindow?.()
      await vi.waitFor(() => {
        expect(resolveSelectWindow).not.toBeNull()
      })
    })

    // After the server confirms (window 1 now active), refreshInspector
    // clears the override and resets selectInFlightRef
    await act(async () => {
      await result.current.refreshInspector('dev')
    })
    expect(result.current.activeWindowIndexOverride).toBeNull()
  })
})

describe('useInspector – selectPane', () => {
  it('preserves pane override until backend confirms pane selection', async () => {
    let resolveSelectPane: (() => void) | null = null
    let serverConfirmed = false
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        return Promise.resolve({
          windows: [makeWindow({ index: 0, active: true })],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({
              windowIndex: 0,
              paneId: '%1',
              active: !serverConfirmed,
            }),
            makePane({
              windowIndex: 0,
              paneId: '%2',
              paneIndex: 1,
              active: serverConfirmed,
            }),
          ],
        })
      }
      return new Promise<void>((resolve) => {
        resolveSelectPane = resolve
      })
    }) as unknown as ApiFunction

    const opts = createMockOptions({
      api,
      windows: [makeWindow({ index: 0, active: true })],
      panes: [
        makePane({ windowIndex: 0, paneId: '%1', active: true }),
        makePane({ windowIndex: 0, paneId: '%2', paneIndex: 1, active: false }),
      ],
    })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.panes.length).toBe(2)
    })

    act(() => {
      result.current.selectPane('%2')
    })
    expect(result.current.activePaneIDOverride).toBe('%2')

    await act(async () => {
      await result.current.refreshInspector('dev')
    })
    expect(result.current.activePaneIDOverride).toBe('%2')

    await act(async () => {
      serverConfirmed = true
      resolveSelectPane?.()
      await vi.waitFor(() => {
        expect(resolveSelectPane).not.toBeNull()
      })
    })

    await act(async () => {
      await result.current.refreshInspector('dev')
    })
    expect(result.current.activePaneIDOverride).toBeNull()
  })
})

describe('useInspector – optimistic createWindow', () => {
  it('seeds a pending pane placeholder for the new window', async () => {
    const never = new Promise<void>(() => {})
    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        return Promise.resolve({
          windows: [
            makeWindow({ index: 0, active: true }),
            makeWindow({ index: 1, name: 'alt', active: false }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({ windowIndex: 0, paneId: '%1', active: true }),
            makePane({ windowIndex: 1, paneId: '%2', active: false }),
          ],
        })
      }
      if (typeof url === 'string' && url.includes('/new-window')) {
        return never
      }
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows.length).toBe(2)
    })

    act(() => {
      result.current.createWindow()
    })

    const pendingPaneID = buildPendingSplitPaneID('dev', 2, 0)
    expect(result.current.activeWindowIndexOverride).toBe(2)
    expect(result.current.activePaneIDOverride).toBe(pendingPaneID)
    expect(
      result.current.panes.some(
        (paneInfo) =>
          paneInfo.paneId === pendingPaneID &&
          paneInfo.windowIndex === 2 &&
          paneInfo.active,
      ),
    ).toBe(true)
  })
})

describe('useInspector – session switch hygiene', () => {
  it('closes rename dialogs when the active session changes', async () => {
    let currentActiveSession = 'dev'
    const opts = createMockOptions()
    const { wrapper } = createWrapper()
    const { result, rerender } = renderHook(
      () => useInspector({ ...opts, activeSession: currentActiveSession }),
      { wrapper },
    )

    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    act(() => {
      result.current.handleOpenRenameWindow(result.current.windows[0]!)
    })
    expect(result.current.renameWindowDialogOpen).toBe(true)

    currentActiveSession = 'ops'
    opts.tabsStateRef.current.activeSession = 'ops'
    rerender()

    expect(result.current.renameWindowDialogOpen).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Tests — generation invalidation (stale refreshInspector race)
// ---------------------------------------------------------------------------

describe('useInspector – selectWindow invalidates stale refreshInspector', () => {
  it('discards a refreshInspector that started before selectWindow', async () => {
    // Track fetch calls to /windows so we can control timing
    let windowsFetchCount = 0
    let resolveSlowWindows: ((v: unknown) => void) | null = null

    const api = vi.fn((url: string) => {
      if (typeof url === 'string' && url.includes('/windows')) {
        windowsFetchCount++
        if (windowsFetchCount <= 1) {
          // First call (initial refresh) — resolve immediately
          return Promise.resolve({
            windows: [
              makeWindow({ index: 0, active: true }),
              makeWindow({ index: 1, name: 'alt', active: false }),
            ],
          })
        }
        // Second call (the stale refresh) — delay so we can interleave
        return new Promise((resolve) => {
          resolveSlowWindows = resolve
        })
      }
      if (typeof url === 'string' && url.includes('/panes')) {
        return Promise.resolve({
          panes: [
            makePane({ windowIndex: 0, paneId: '%1', active: true }),
            makePane({ windowIndex: 1, paneId: '%2', active: false }),
          ],
        })
      }
      // select-window — resolve immediately
      return Promise.resolve(undefined)
    }) as unknown as ApiFunction

    const opts = createMockOptions({ api })
    const { wrapper } = createWrapper()
    const { result } = renderHook(() => useInspector(opts), { wrapper })

    // Wait for initial refresh to complete
    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    // Start a refreshInspector — it will hang on the /windows fetch
    let refreshPromise: Promise<void> | undefined
    act(() => {
      refreshPromise = result.current.refreshInspector('dev', {
        background: true,
      })
    })

    // While the refresh is awaiting /windows, user clicks selectWindow(1).
    // This bumps inspectorGenerationRef, invalidating the in-flight refresh.
    act(() => {
      result.current.selectWindow(1)
    })
    expect(result.current.activeWindowIndexOverride).toBe(1)

    // Now resolve the slow /windows fetch — the stale refresh should bail
    // when it checks the generation, leaving the override intact.
    await act(async () => {
      resolveSlowWindows?.({
        windows: [
          makeWindow({ index: 0, active: true }),
          makeWindow({ index: 1, name: 'alt', active: false }),
        ],
      })
      await refreshPromise
    })

    // Override MUST be preserved — the stale refresh was discarded
    expect(result.current.activeWindowIndexOverride).toBe(1)
  })
})

// ---------------------------------------------------------------------------
// Tests — applyInspectorProjectionPatches override clearing
// ---------------------------------------------------------------------------

describe('useInspector – applyInspectorProjectionPatches', () => {
  it('clears override when overridden window is removed', async () => {
    const opts = createMockOptions()
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    // Set override to window index 1
    act(() => {
      result.current.setActiveWindowIndexOverride(1)
    })
    expect(result.current.activeWindowIndexOverride).toBe(1)

    // Apply a patch that removes window index 1
    act(() => {
      result.current.applyInspectorProjectionPatches([
        {
          session: 'dev',
          windows: [
            {
              session: 'dev',
              index: 0,
              name: 'main',
              active: true,
              panes: 1,
            },
          ],
          panes: [
            {
              session: 'dev',
              windowIndex: 0,
              paneIndex: 0,
              paneId: '%1',
              title: 'shell',
              active: true,
              tty: '/dev/pts/1',
            },
          ],
        },
      ])
    })

    // Override should be cleared — the target window no longer exists
    expect(result.current.activeWindowIndexOverride).toBeNull()
  })

  it('keeps override when overridden window still exists', async () => {
    const opts = createMockOptions()
    const { wrapper } = createWrapper()

    const { result } = renderHook(() => useInspector(opts), { wrapper })

    await waitFor(() => {
      expect(result.current.windows.length).toBeGreaterThan(0)
    })

    // Set override to window index 1
    act(() => {
      result.current.setActiveWindowIndexOverride(1)
    })

    // Apply a patch where window 1 still exists but the server says window
    // 0 is active (divergence). Override should NOT be cleared by patches.
    act(() => {
      result.current.applyInspectorProjectionPatches([
        {
          session: 'dev',
          windows: [
            {
              session: 'dev',
              index: 0,
              name: 'main',
              active: true,
              panes: 1,
            },
            {
              session: 'dev',
              index: 1,
              name: 'alt',
              active: false,
              panes: 1,
            },
          ],
          panes: [
            {
              session: 'dev',
              windowIndex: 0,
              paneIndex: 0,
              paneId: '%1',
              title: 'shell',
              active: true,
              tty: '/dev/pts/1',
            },
            {
              session: 'dev',
              windowIndex: 1,
              paneIndex: 0,
              paneId: '%2',
              title: 'shell',
              active: false,
              tty: '/dev/pts/2',
            },
          ],
        },
      ])
    })

    // Override should be kept — the window still exists
    expect(result.current.activeWindowIndexOverride).toBe(1)
  })
})
