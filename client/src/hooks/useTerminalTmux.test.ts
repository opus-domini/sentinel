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
  return {
    Terminal: class {
      options: Record<string, unknown> = {}
      element: HTMLDivElement | null = null
      textarea: HTMLTextAreaElement | null = null
      cols = 80
      rows = 24
      buffer = { active: { type: 'normal' } }
      onData = vi.fn(() => ({ dispose: _noop }))
      onResize = vi.fn(() => ({ dispose: _noop }))
      onSelectionChange = vi.fn(() => ({ dispose: _noop }))
      loadAddon = vi.fn()
      open = vi.fn()
      reset = vi.fn()
      focus = vi.fn()
      write = vi.fn()
      getSelection = vi.fn(() => '')
      scrollToBottom = vi.fn()
      dispose = vi.fn()
      attachCustomWheelEventHandler = vi.fn()
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
vi.mock('@xterm/addon-webgl', () => ({
  WebglAddon: class {
    onContextLoss = vi.fn(() => ({ dispose() {} }))
    dispose() {}
  },
}))

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
  static readonly OPEN = 1

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
    this.onclose?.(new CloseEvent('close'))
  }

  send = vi.fn()

  emitOpen() {
    this.onopen?.(new Event('open'))
  }

  emitClose() {
    this.onclose?.(new CloseEvent('close'))
  }
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('useTerminalTmux – setConnection guard', () => {
  const originalWebSocket = globalThis.WebSocket

  beforeEach(() => {
    MockWebSocket.instances = []
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket

    // Ensure localStorage is available (Node 22+ needs --localstorage-file)
    if (typeof globalThis.localStorage === 'undefined' || typeof globalThis.localStorage.getItem !== 'function') {
      const store = new Map<string, string>()
      Object.defineProperty(globalThis, 'localStorage', {
        value: {
          getItem: (key: string) => store.get(key) ?? null,
          setItem: (key: string, value: string) => store.set(key, value),
          removeItem: (key: string) => store.delete(key),
          clear: () => store.clear(),
          get length() { return store.size },
          key: () => null,
        },
        writable: true,
        configurable: true,
      })
    }

    // document.fonts.ready is used in openRuntimeInHost
    Object.defineProperty(document, 'fonts', {
      value: { ready: Promise.resolve() },
      writable: true,
      configurable: true,
    })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

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

  function connectSession() {
    const ws = MockWebSocket.instances[MockWebSocket.instances.length - 1]
    act(() => {
      ws.emitOpen()
    })
    return ws
  }

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

    // Switch back to session A — it should still be connected.
    rerender({
      openTabs: ['session-a', 'session-b'],
      activeSession: 'session-a',
      activeEpoch: 2,
    })

    expect(result.current.connectionState).toBe('connected')
    expect(result.current.statusDetail).toBe('attached session-a')
  })
})
