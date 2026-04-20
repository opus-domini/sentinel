// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useTerminalTmux } from './useTerminalTmux'

// ---------------------------------------------------------------------------
// Mock all heavy xterm.js / addon imports with minimal stubs.
// vi.mock factories are hoisted above all declarations, so everything
// must be self-contained (no references to outer variables).
// ---------------------------------------------------------------------------

vi.mock('@xterm/xterm', () => {
  const _noop = () => undefined
  const instancesKey = '__SENTINEL_TERMINAL_INSTANCES'
  ;(
    globalThis as typeof globalThis & {
      [instancesKey]?: Array<unknown>
    }
  )[instancesKey] = []
  return {
    Terminal: class {
      options: Record<string, unknown> = {}
      element: HTMLDivElement | null = null
      textarea: HTMLTextAreaElement | null = null
      cols = 80
      rows = 24
      unicode: { activeVersion: string } = { activeVersion: '6' }
      buffer = { active: { type: 'normal' } }
      onData = vi.fn(() => ({ dispose: _noop }))
      onResize = vi.fn(() => ({ dispose: _noop }))
      onSelectionChange = vi.fn(() => ({ dispose: _noop }))
      loadAddon = vi.fn()
      open = vi.fn((host: HTMLElement) => {
        const element = document.createElement('div')
        element.className = 'xterm'
        const screen = document.createElement('div')
        screen.className = 'xterm-screen'
        const viewport = document.createElement('div')
        viewport.className = 'xterm-viewport'
        const textarea = document.createElement('textarea')
        element.append(screen, viewport, textarea)
        host.appendChild(element)
        this.element = element
        this.textarea = textarea
      })
      reset = vi.fn()
      focus = vi.fn()
      write = vi.fn()
      getSelection = vi.fn(() => '')
      scrollToBottom = vi.fn()
      dispose = vi.fn()
      clearTextureAtlas = vi.fn()
      attachCustomWheelEventHandler = vi.fn()

      constructor() {
        ;(
          globalThis as typeof globalThis & {
            __SENTINEL_TERMINAL_INSTANCES?: Array<unknown>
          }
        ).__SENTINEL_TERMINAL_INSTANCES?.push(this)
      }
    },
  }
})
vi.mock('@xterm/addon-clipboard', () => ({
  ClipboardAddon: class {
    dispose() {}
  },
}))
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
    dispose() {}
  },
}))
vi.mock('@xterm/addon-search', () => ({
  SearchAddon: class {
    dispose() {}
  },
}))
vi.mock('@xterm/addon-serialize', () => ({
  SerializeAddon: class {
    dispose() {}
  },
}))
vi.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: class {
    dispose() {}
  },
}))
vi.mock('@xterm/addon-unicode-graphemes', () => ({
  UnicodeGraphemesAddon: class {
    dispose() {}
  },
}))
vi.mock('@xterm/addon-webgl', () => {
  const instancesKey = '__SENTINEL_WEBGL_ADDON_INSTANCES'
  ;(
    globalThis as typeof globalThis & {
      [instancesKey]?: Array<unknown>
    }
  )[instancesKey] = []
  return {
    WebglAddon: class {
      contextLossCallbacks: Array<() => void> = []
      atlasGrowthCallbacks: Array<(canvas: HTMLCanvasElement) => void> = []
      onContextLoss = vi.fn((cb: () => void) => {
        this.contextLossCallbacks.push(cb)
        return { dispose() {} }
      })
      onAddTextureAtlasCanvas = vi.fn(
        (cb: (canvas: HTMLCanvasElement) => void) => {
          this.atlasGrowthCallbacks.push(cb)
          return { dispose() {} }
        },
      )
      dispose = vi.fn()

      constructor() {
        ;(
          globalThis as typeof globalThis & {
            __SENTINEL_WEBGL_ADDON_INSTANCES?: Array<unknown>
          }
        ).__SENTINEL_WEBGL_ADDON_INSTANCES?.push(this)
      }
    },
  }
})

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: vi.fn() }),
}))
vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))
vi.mock('@/lib/clipboardProvider', () => ({
  createWebClipboardProvider: () => ({
    readText: () => Promise.resolve(''),
    writeText: () => Promise.resolve(),
  }),
  writeClipboardText: () => undefined,
}))
vi.mock('@/lib/touchWheelBridge', () => ({
  attachTouchWheelBridge: () => ({ dispose: () => undefined }),
}))
vi.mock('@/lib/wsAuth', () => ({
  buildWSProtocols: () => ['sentinel.v1'],
}))

// ---------------------------------------------------------------------------
// MockWebSocket — same pattern as other WS tests in the project
// ---------------------------------------------------------------------------

class MockWebSocket {
  static instances: Array<MockWebSocket> = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  url: string
  protocols: Array<string> | string | undefined
  binaryType = 'blob'
  readyState = MockWebSocket.OPEN

  onopen: ((ev: Event) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null

  constructor(url: string, protocols?: Array<string> | string) {
    this.url = url
    this.protocols = protocols
    MockWebSocket.instances.push(this)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }

  send = vi.fn()

  emitOpen() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  emitClose() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

const originalWebSocket = globalThis.WebSocket

function setupEnvironment() {
  MockWebSocket.instances = []
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_TERMINAL_INSTANCES?: Array<unknown>
    }
  ).__SENTINEL_TERMINAL_INSTANCES = []
  resetWebglAddonInstances()
  globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
  document.documentElement.style.setProperty('--surface-inset', '#112233')
  document.documentElement.style.setProperty('--foreground', '#ddeeff')
  document.documentElement.style.setProperty('--link', '#88aaff')

  if (
    typeof globalThis.localStorage === 'undefined' ||
    typeof globalThis.localStorage.getItem !== 'function'
  ) {
    const store = new Map<string, string>()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: (key: string) => store.get(key) ?? null,
        setItem: (key: string, value: string) => store.set(key, value),
        removeItem: (key: string) => store.delete(key),
        clear: () => store.clear(),
        get length() {
          return store.size
        },
        key: () => null,
      },
      writable: true,
      configurable: true,
    })
  }

  Object.defineProperty(document, 'fonts', {
    value: { ready: Promise.resolve() },
    writable: true,
    configurable: true,
  })
}

function latestWebglAddon() {
  return (
    (
      globalThis as typeof globalThis & {
        __SENTINEL_WEBGL_ADDON_INSTANCES?: Array<{
          contextLossCallbacks: Array<() => void>
          atlasGrowthCallbacks: Array<(canvas: HTMLCanvasElement) => void>
          dispose: ReturnType<typeof vi.fn>
        }>
      }
    ).__SENTINEL_WEBGL_ADDON_INSTANCES?.at(-1) ?? null
  )
}

function resetWebglAddonInstances() {
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_WEBGL_ADDON_INSTANCES?: Array<unknown>
    }
  ).__SENTINEL_WEBGL_ADDON_INSTANCES = []
}

function latestTerminal() {
  return (
    (
      globalThis as typeof globalThis & {
        __SENTINEL_TERMINAL_INSTANCES?: Array<{
          clearTextureAtlas: ReturnType<typeof vi.fn>
        }>
      }
    ).__SENTINEL_TERMINAL_INSTANCES?.at(-1) ?? null
  )
}

function terminalInstances() {
  return (
    (
      globalThis as typeof globalThis & {
        __SENTINEL_TERMINAL_INSTANCES?: Array<{
          clearTextureAtlas: ReturnType<typeof vi.fn>
        }>
      }
    ).__SENTINEL_TERMINAL_INSTANCES ?? []
  )
}

function renderTerminalHook(
  overrides: {
    openTabs?: Array<string>
    activeSession?: string
    activeEpoch?: number
  } = {},
) {
  const props = {
    openTabs: overrides.openTabs ?? ['test-session'],
    activeSession: overrides.activeSession ?? 'test-session',
    activeEpoch: overrides.activeEpoch ?? 0,
    sidebarCollapsed: false,
    onAttachedMobile: vi.fn(),
  }
  return renderHook(
    ({ openTabs, activeSession, activeEpoch }) =>
      useTerminalTmux({
        openTabs,
        activeSession,
        activeEpoch,
        sidebarCollapsed: props.sidebarCollapsed,
        onAttachedMobile: props.onAttachedMobile,
      }),
    { initialProps: props },
  )
}

function latestWS(): MockWebSocket {
  return MockWebSocket.instances[MockWebSocket.instances.length - 1]
}

function connectSession() {
  const ws = latestWS()
  act(() => {
    ws.emitOpen()
  })
  return ws
}

// ---------------------------------------------------------------------------
// setConnection guard
// ---------------------------------------------------------------------------

describe('useTerminalTmux – setConnection guard', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('setConnection does not regress connected → connecting', () => {
    const { result } = renderTerminalHook()
    connectSession()

    expect(result.current.connectionState).toBe('connected')

    act(() => {
      result.current.setConnection('connecting', 'creating new-session')
    })

    // Must remain 'connected' — the guard prevents regression.
    expect(result.current.connectionState).toBe('connected')
  })

  it('setConnection allows connected → disconnected', () => {
    const { result } = renderTerminalHook()
    connectSession()

    expect(result.current.connectionState).toBe('connected')

    act(() => {
      result.current.setConnection('disconnected', 'session killed')
    })

    expect(result.current.connectionState).toBe('disconnected')
  })

  it('setConnection allows connected → error', () => {
    const { result } = renderTerminalHook()
    connectSession()

    expect(result.current.connectionState).toBe('connected')

    act(() => {
      result.current.setConnection('error', 'something went wrong')
    })

    expect(result.current.connectionState).toBe('error')
  })

  it('creating a new session does not poison existing runtime', () => {
    // Start with session A connected
    const { result, rerender } = renderTerminalHook({
      openTabs: ['session-a'],
      activeSession: 'session-a',
      activeEpoch: 0,
    })

    // Connect session A
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Now add session B and make it active (simulates session creation)
    rerender({
      openTabs: ['session-a', 'session-b'],
      activeSession: 'session-b',
      activeEpoch: 1,
    })

    // Session B should create a new WS. Connect it.
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Switch back to session A — its background socket was closed,
    // so it reconnects on activation.
    rerender({
      openTabs: ['session-a', 'session-b'],
      activeSession: 'session-a',
      activeEpoch: 2,
    })

    connectSession()
    expect(result.current.connectionState).toBe('connected')
    expect(result.current.statusDetail).toBe('attached session-a')
  })

  it('retries attach on active epoch bump when the current socket is not open', () => {
    const { rerender } = renderTerminalHook({
      openTabs: ['session-b'],
      activeSession: 'session-b',
      activeEpoch: 0,
    })

    const firstSocket = latestWS()
    Object.defineProperty(firstSocket, 'readyState', {
      value: 3,
      configurable: true,
    })

    rerender({
      openTabs: ['session-b'],
      activeSession: 'session-b',
      activeEpoch: 1,
    })

    expect(MockWebSocket.instances).toHaveLength(2)
    expect(latestWS()).not.toBe(firstSocket)
  })

  it('does not retry attach on active epoch bump while the socket is still connecting', () => {
    const { rerender } = renderTerminalHook({
      openTabs: ['session-b'],
      activeSession: 'session-b',
      activeEpoch: 0,
    })

    const firstSocket = latestWS()
    Object.defineProperty(firstSocket, 'readyState', {
      value: 0,
      configurable: true,
    })

    rerender({
      openTabs: ['session-b'],
      activeSession: 'session-b',
      activeEpoch: 1,
    })

    expect(MockWebSocket.instances).toHaveLength(1)
    expect(latestWS()).toBe(firstSocket)
  })
})

// ---------------------------------------------------------------------------
// Auto-reconnect on unexpected socket close
// ---------------------------------------------------------------------------

describe('useTerminalTmux – auto-reconnect', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setupEnvironment()
  })

  afterEach(() => {
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
  })

  it('schedules reconnect on unexpected socket close', () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    // Simulate unexpected close (no manualCloseReason)
    act(() => {
      ws.emitClose()
    })

    // State should be 'connecting' (yellow badge), not 'disconnected'
    expect(result.current.connectionState).toBe('connecting')
    expect(result.current.statusDetail).toMatch(/reconnecting in \d+s/)

    // No new WebSocket yet — waiting for the timer
    expect(MockWebSocket.instances.length).toBe(countBefore)

    // Advance past the first reconnect delay (1200ms)
    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // A new WebSocket should have been created
    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
    expect(result.current.connectionState).toBe('connecting')
  })

  it('transitions to connected after successful reconnect', () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Unexpected close → reconnect scheduled
    act(() => {
      ws.emitClose()
    })
    expect(result.current.connectionState).toBe('connecting')

    // Advance timer to trigger reconnect
    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // Complete the reconnect handshake
    connectSession()
    expect(result.current.connectionState).toBe('connected')
    expect(result.current.statusDetail).toBe('attached test-session')
  })

  it('does NOT reconnect on manual close (tab closed)', () => {
    const { result } = renderTerminalHook()
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    // Manual close via closeCurrentSocket
    act(() => {
      result.current.closeCurrentSocket('tab closed')
    })

    expect(result.current.connectionState).toBe('disconnected')
    expect(result.current.statusDetail).toBe('tab closed')

    // Advance well past any potential reconnect delay
    act(() => {
      vi.advanceTimersByTime(60_000)
    })

    // No new WebSocket should have been created
    expect(MockWebSocket.instances.length).toBe(countBefore)
  })

  it('does NOT reconnect when tab is removed from openTabs', () => {
    const { result, rerender } = renderTerminalHook({
      openTabs: ['sess-a'],
      activeSession: 'sess-a',
    })
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    // Remove session from openTabs (dispose path)
    rerender({
      openTabs: [],
      activeSession: '',
      activeEpoch: 1,
    })

    // Advance well past any potential reconnect delay
    act(() => {
      vi.advanceTimersByTime(60_000)
    })

    // No new WebSocket should have been created after disposal
    expect(MockWebSocket.instances.length).toBe(countBefore)
  })

  it('applies exponential backoff on repeated failures', () => {
    const { result } = renderTerminalHook()
    connectSession()

    // First unexpected close
    act(() => {
      latestWS().emitClose()
    })
    expect(result.current.statusDetail).toBe('reconnecting in 2s') // ceil(1200/1000)

    // Advance first timer → new WS created, open it, then close again
    act(() => {
      vi.advanceTimersByTime(1_200)
    })
    act(() => {
      latestWS().emitClose() // close immediately (failed to connect)
    })

    // Second backoff: ceil(2040/1000) = 3s
    expect(result.current.statusDetail).toBe('reconnecting in 3s')

    // Advance second timer → new WS, close again
    act(() => {
      vi.advanceTimersByTime(2_040)
    })
    act(() => {
      latestWS().emitClose()
    })

    // Third backoff: ceil(3468/1000) = 4s
    expect(result.current.statusDetail).toBe('reconnecting in 4s')
  })

  it('resets backoff after a successful reconnection', () => {
    const { result } = renderTerminalHook()
    connectSession()

    // Unexpected close → first backoff
    act(() => {
      latestWS().emitClose()
    })

    // Advance timer to trigger reconnect
    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // Fail again (close before open)
    act(() => {
      latestWS().emitClose()
    })

    // Now at second backoff level (2040ms)
    act(() => {
      vi.advanceTimersByTime(2_040)
    })

    // This time, the reconnect succeeds
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Close unexpectedly again — backoff should be reset to initial
    act(() => {
      latestWS().emitClose()
    })
    expect(result.current.statusDetail).toBe('reconnecting in 2s') // ceil(1200/1000), back to initial
  })

  it('cancels pending reconnect when closeCurrentSocket is called', () => {
    const { result } = renderTerminalHook()
    connectSession()

    // Unexpected close → reconnect scheduled
    act(() => {
      latestWS().emitClose()
    })
    expect(result.current.connectionState).toBe('connecting')

    const countBefore = MockWebSocket.instances.length

    // Manual intervention before the timer fires
    act(() => {
      result.current.closeCurrentSocket('user action')
    })

    // Advance past the reconnect delay
    act(() => {
      vi.advanceTimersByTime(60_000)
    })

    // No new WebSocket — the timer was cancelled
    expect(MockWebSocket.instances.length).toBe(countBefore)
    expect(result.current.connectionState).toBe('disconnected')
  })

  it('reconnects after onerror + onclose sequence', () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()

    const countBefore = MockWebSocket.instances.length

    // Simulate error followed by close (typical browser behavior)
    act(() => {
      ws.onerror?.(new Event('error'))
    })
    expect(result.current.connectionState).toBe('error')

    act(() => {
      ws.emitClose()
    })

    // Should schedule reconnect, not stay in 'error'
    expect(result.current.connectionState).toBe('connecting')
    expect(result.current.statusDetail).toMatch(/reconnecting/)

    // Advance timer
    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // New WebSocket created
    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
  })

  it('forces reconnect when the websocket handshake stalls in CONNECTING', () => {
    const { result } = renderTerminalHook()
    const ws = latestWS()

    Object.defineProperty(ws, 'readyState', {
      value: MockWebSocket.CONNECTING,
      configurable: true,
    })

    const countBefore = MockWebSocket.instances.length

    act(() => {
      vi.advanceTimersByTime(7_000)
    })

    expect(result.current.connectionState).toBe('connecting')
    expect(result.current.statusDetail).toBe('reconnecting in 2s')

    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
  })
})

// ---------------------------------------------------------------------------
// reconnectActiveSession – force parameter
// ---------------------------------------------------------------------------

describe('useTerminalTmux – reconnectActiveSession force', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('reconnectActiveSession() without force skips when socket is OPEN', () => {
    const { result } = renderTerminalHook()
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    act(() => {
      result.current.reconnectActiveSession()
    })

    // No new WebSocket — the OPEN guard prevented reconnection.
    expect(MockWebSocket.instances.length).toBe(countBefore)
    expect(result.current.connectionState).toBe('connected')
  })

  it('reconnectActiveSession() without force skips when socket is still CONNECTING', () => {
    const { result } = renderTerminalHook()

    const countBefore = MockWebSocket.instances.length

    act(() => {
      result.current.reconnectActiveSession()
    })

    expect(MockWebSocket.instances.length).toBe(countBefore)
    expect(result.current.connectionState).toBe('connecting')
  })

  it('reconnectActiveSession({ force: true }) reconnects even when socket is OPEN', () => {
    const { result } = renderTerminalHook()
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    act(() => {
      result.current.reconnectActiveSession({ force: true })
    })

    // A new WebSocket was created despite the old one being OPEN.
    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
    expect(result.current.connectionState).toBe('connecting')
  })

  it('reconnectActiveSession({ force: true }) reconnects while socket is CONNECTING', () => {
    const { result } = renderTerminalHook()

    const countBefore = MockWebSocket.instances.length

    act(() => {
      result.current.reconnectActiveSession({ force: true })
    })

    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
    expect(result.current.connectionState).toBe('connecting')
  })

  it('reconnectActiveSession resets backoff before connecting', () => {
    vi.useFakeTimers()
    const { result } = renderTerminalHook()
    connectSession()

    // Trigger unexpected close to advance the backoff
    act(() => {
      latestWS().emitClose()
    })
    // Advance timer to trigger first reconnect
    act(() => {
      vi.advanceTimersByTime(1_200)
    })
    // Close again to advance backoff further
    act(() => {
      latestWS().emitClose()
    })

    // Now backoff is at 2040ms (second level).
    // Call reconnectActiveSession — it should reset the backoff.
    act(() => {
      result.current.reconnectActiveSession()
    })

    // Connect successfully
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Close unexpectedly again — backoff should be at initial level
    // because reconnectActiveSession reset it, and onopen reset it again.
    act(() => {
      latestWS().emitClose()
    })
    expect(result.current.statusDetail).toBe('reconnecting in 2s') // ceil(1200/1000)

    vi.useRealTimers()
  })
})

// ---------------------------------------------------------------------------
// visibilitychange reconnection
// ---------------------------------------------------------------------------

describe('useTerminalTmux – visibilitychange reconnection', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('reconnects when document becomes visible and socket is not OPEN', () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()
    expect(result.current.connectionState).toBe('connected')

    // Simulate zombie socket: force readyState to CLOSED without triggering
    // the onclose handler (mimics TCP-dead scenario).
    Object.defineProperty(ws, 'readyState', { value: 3 }) // WebSocket.CLOSED

    // Simulate returning to the tab
    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    // A new WebSocket should have been created — the hook reconnected.
    const lastWS = latestWS()
    expect(lastWS).not.toBe(ws)
    expect(lastWS.url).toContain('session=test-session')
    expect(result.current.connectionState).toBe('connecting')
  })

  it('does not reconnect on visibilitychange when socket is OPEN', () => {
    const { result } = renderTerminalHook()
    connectSession()
    expect(result.current.connectionState).toBe('connected')

    const countBefore = MockWebSocket.instances.length

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    // No new WebSocket — already connected.
    expect(MockWebSocket.instances.length).toBe(countBefore)
  })

  it('does not reconnect on visibilitychange while socket is still connecting', () => {
    renderTerminalHook()
    const ws = latestWS()
    Object.defineProperty(ws, 'readyState', {
      value: 0,
      configurable: true,
    })
    const countBefore = MockWebSocket.instances.length

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    expect(MockWebSocket.instances.length).toBe(countBefore)
    expect(latestWS()).toBe(ws)
  })

  it('ignores visibilitychange when document becomes hidden', () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()
    expect(result.current.connectionState).toBe('connected')

    Object.defineProperty(ws, 'readyState', { value: WebSocket.CLOSED })
    const countBefore = MockWebSocket.instances.length

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'hidden',
        writable: true,
        configurable: true,
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    // No reconnection on hidden.
    expect(MockWebSocket.instances.length).toBe(countBefore)
  })

  it('clears the texture atlas when the window regains focus', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.clearTextureAtlas.mockClear()

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      window.dispatchEvent(new Event('focus'))
    })

    expect(terminal?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    host.remove()
  })
})

// ---------------------------------------------------------------------------
// Terminal chrome
// ---------------------------------------------------------------------------

describe('useTerminalTmux – terminal chrome', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('applies themed host gutter and keeps it in sync on theme change', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminalRoot = host.querySelector<HTMLElement>('.xterm')
    expect(host.style.paddingInlineStart).toBe('8px')
    expect(host.style.boxSizing).toBe('border-box')
    expect(host.style.backgroundColor).not.toBe('')
    expect(terminalRoot?.style.backgroundColor).toBe(host.style.backgroundColor)

    const initialBackground = host.style.backgroundColor

    await act(async () => {
      latestTerminal()?.clearTextureAtlas.mockClear()
      window.dispatchEvent(
        new CustomEvent('sentinel-theme-change', { detail: 'dracula' }),
      )
      await Promise.resolve()
    })

    expect(host.style.backgroundColor).not.toBe(initialBackground)
    expect(terminalRoot?.style.backgroundColor).toBe(host.style.backgroundColor)
    expect(latestTerminal()?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    host.remove()
  })

  it('skips the WebGL addon when WebGL2 is unavailable', () => {
    const originalWebGL2RenderingContext = globalThis.WebGL2RenderingContext
    Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
      value: undefined,
      writable: true,
      configurable: true,
    })

    try {
      renderTerminalHook()
      expect(latestTerminal()?.loadAddon).toHaveBeenCalledTimes(6)
    } finally {
      Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
        value: originalWebGL2RenderingContext,
        writable: true,
        configurable: true,
      })
    }
  })

  it('assigns a name to the hidden terminal textarea', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    expect(host.querySelector('textarea')?.getAttribute('name')).toBe(
      'terminal-input-test-session',
    )

    host.remove()
  })

  it('activates the unicode-graphemes provider so emoji widths match tmux', () => {
    renderTerminalHook()
    expect(latestTerminal()?.unicode.activeVersion).toBe('15-graphemes')
  })
})

describe('useTerminalTmux – webgl context loss', () => {
  let originalWebGL2: unknown
  let originalGetContext: typeof HTMLCanvasElement.prototype.getContext

  beforeEach(() => {
    setupEnvironment()
    originalWebGL2 = (globalThis as { WebGL2RenderingContext?: unknown })
      .WebGL2RenderingContext
    Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
      value: function WebGL2RenderingContext() {},
      writable: true,
      configurable: true,
    })
    originalGetContext = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function stubGetContext(
      this: HTMLCanvasElement,
      type: string,
    ) {
      if (type === 'webgl2') return {} as RenderingContext
      return null
    } as typeof HTMLCanvasElement.prototype.getContext
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    HTMLCanvasElement.prototype.getContext = originalGetContext
    Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
      value: originalWebGL2,
      writable: true,
      configurable: true,
    })
  })

  it('disposes the WebGL addon on context loss so xterm falls back to DOM', () => {
    renderTerminalHook()
    const addon = latestWebglAddon()
    expect(addon).not.toBeNull()
    expect(addon?.dispose).not.toHaveBeenCalled()

    act(() => {
      for (const cb of addon?.contextLossCallbacks ?? []) {
        cb()
      }
    })

    expect(addon?.dispose).toHaveBeenCalledTimes(1)
  })

  it('guards against a second context-loss firing after dispose', () => {
    renderTerminalHook()
    const addon = latestWebglAddon()
    const cb = addon?.contextLossCallbacks[0]
    expect(cb).toBeDefined()

    act(() => {
      cb?.()
      cb?.()
    })

    expect(addon?.dispose).toHaveBeenCalledTimes(1)
  })
})

describe('useTerminalTmux – webgl atlas growth', () => {
  let originalWebGL2: unknown
  let originalGetContext: typeof HTMLCanvasElement.prototype.getContext

  beforeEach(() => {
    vi.useFakeTimers()
    setupEnvironment()
    originalWebGL2 = (globalThis as { WebGL2RenderingContext?: unknown })
      .WebGL2RenderingContext
    Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
      value: function WebGL2RenderingContext() {},
      writable: true,
      configurable: true,
    })
    originalGetContext = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function stubGetContext(
      this: HTMLCanvasElement,
      type: string,
    ) {
      if (type === 'webgl2') return {} as RenderingContext
      return null
    } as typeof HTMLCanvasElement.prototype.getContext
  })

  afterEach(() => {
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
    HTMLCanvasElement.prototype.getContext = originalGetContext
    Object.defineProperty(globalThis, 'WebGL2RenderingContext', {
      value: originalWebGL2,
      writable: true,
      configurable: true,
    })
  })

  it('does not clear atlas for the first two page additions', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)
    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.clearTextureAtlas.mockClear()

    const addon = latestWebglAddon()
    const growthCb = addon?.atlasGrowthCallbacks[0]
    expect(growthCb).toBeDefined()

    const fakeCanvas = document.createElement('canvas')
    act(() => {
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
    })

    act(() => {
      vi.advanceTimersByTime(1_000)
    })

    expect(terminal?.clearTextureAtlas).not.toHaveBeenCalled()

    host.remove()
  })

  it('debounces an atlas clear once the third page is added', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)
    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.clearTextureAtlas.mockClear()

    const addon = latestWebglAddon()
    const growthCb = addon?.atlasGrowthCallbacks[0]
    const fakeCanvas = document.createElement('canvas')

    act(() => {
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas) // burst — additional events collapse into one clear
    })

    expect(terminal?.clearTextureAtlas).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(250)
    })

    expect(terminal?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    host.remove()
  })

  it('resets the page counter after a clear so growth can be detected again', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)
    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.clearTextureAtlas.mockClear()

    const addon = latestWebglAddon()
    const growthCb = addon?.atlasGrowthCallbacks[0]
    const fakeCanvas = document.createElement('canvas')

    act(() => {
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
    })
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(terminal?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    // Second wave must not clear until 3 fresh pages were added.
    act(() => {
      growthCb?.(fakeCanvas)
      growthCb?.(fakeCanvas)
    })
    act(() => {
      vi.advanceTimersByTime(1_000)
    })
    expect(terminal?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    act(() => {
      growthCb?.(fakeCanvas)
    })
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(terminal?.clearTextureAtlas).toHaveBeenCalledTimes(2)

    host.remove()
  })
})

describe('useTerminalTmux – renderer refresh', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('clears the newly active session atlas when switching tabs', async () => {
    const { result, rerender } = renderTerminalHook({
      openTabs: ['session-a', 'session-b'],
      activeSession: 'session-a',
      activeEpoch: 0,
    })
    const hostA = document.createElement('div')
    const hostB = document.createElement('div')
    document.body.append(hostA, hostB)

    await act(async () => {
      result.current.getTerminalHostRef('session-a')(hostA)
      result.current.getTerminalHostRef('session-b')(hostB)
      await Promise.resolve()
    })

    const terminals = terminalInstances()
    const activeNextTerminal = terminals[1]
    activeNextTerminal?.clearTextureAtlas.mockClear()

    act(() => {
      rerender({
        openTabs: ['session-a', 'session-b'],
        activeSession: 'session-b',
        activeEpoch: 1,
      })
    })

    expect(activeNextTerminal?.clearTextureAtlas).toHaveBeenCalledTimes(1)

    hostA.remove()
    hostB.remove()
  })
})
