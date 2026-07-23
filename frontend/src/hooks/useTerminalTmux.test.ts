// @vitest-environment jsdom
import { act, cleanup, fireEvent, renderHook } from '@testing-library/react'
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
      buffer = { active: { type: 'normal', viewportY: 0, length: 24 } }
      dataHandler: ((data: string) => void) | null = null
      onData = vi.fn((handler: (data: string) => void) => {
        this.dataHandler = handler
        return { dispose: _noop }
      })
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
      refresh = vi.fn()
      getSelection = vi.fn(() => '')
      clearSelection = vi.fn()
      select = vi.fn()
      scrollLines = vi.fn()
      scrollToBottom = vi.fn()
      dispose = vi.fn()
      attachCustomWheelEventHandler = vi.fn()
      emitData = (data: string) => this.dataHandler?.(data)

      constructor(options?: Record<string, unknown>) {
        this.options = options ?? {}
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

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: vi.fn() }),
}))
vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => {
    const enabled =
      (
        globalThis as typeof globalThis & {
          __SENTINEL_IS_MOBILE_LAYOUT?: boolean
        }
      ).__SENTINEL_IS_MOBILE_LAYOUT === true
    return {
      compactLayout: enabled,
      touchCapable: enabled,
      touchOptimized: enabled,
    }
  },
}))
vi.mock('@/lib/clipboardProvider', () => ({
  createWebClipboardProvider: () => ({
    readText: () => Promise.resolve(''),
    writeText: () => Promise.resolve(),
  }),
  writeClipboardText: async (text: string) => {
    ;(
      globalThis as typeof globalThis & {
        __SENTINEL_WRITE_CLIPBOARD_TEXT?: (text: string) => void
      }
    ).__SENTINEL_WRITE_CLIPBOARD_TEXT?.(text)
    return true
  },
}))
vi.mock('@/lib/touchWheelBridge', () => ({
  attachTouchWheelBridge: () => ({ dispose: () => undefined }),
}))
vi.mock('@/lib/touchTerminalSelection', () => ({
  attachTouchTerminalSelection: ({
    onSelectionChange,
  }: {
    onSelectionChange: (hasSelection: boolean) => void
  }) => {
    ;(
      globalThis as typeof globalThis & {
        __SENTINEL_TOUCH_SELECTION_CHANGE?: (hasSelection: boolean) => void
      }
    ).__SENTINEL_TOUCH_SELECTION_CHANGE = onSelectionChange
    return { dispose: () => undefined }
  },
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

afterEach(() => {
  cleanup()
})

function setupEnvironment() {
  MockWebSocket.instances = []
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_TERMINAL_INSTANCES?: Array<unknown>
    }
  ).__SENTINEL_TERMINAL_INSTANCES = []
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_IS_MOBILE_LAYOUT?: boolean
    }
  ).__SENTINEL_IS_MOBILE_LAYOUT = false
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_WRITE_CLIPBOARD_TEXT?: (text: string) => void
    }
  ).__SENTINEL_WRITE_CLIPBOARD_TEXT = vi.fn()
  ;(
    globalThis as typeof globalThis & {
      __SENTINEL_TOUCH_SELECTION_CHANGE?: (hasSelection: boolean) => void
    }
  ).__SENTINEL_TOUCH_SELECTION_CHANGE = undefined
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

type MockTerminalInstance = {
  options: Record<string, unknown>
  textarea: HTMLTextAreaElement | null
  cols: number
  rows: number
  onSelectionChange: ReturnType<typeof vi.fn>
  emitData: (data: string) => void
  getSelection: ReturnType<typeof vi.fn>
  clearSelection: ReturnType<typeof vi.fn>
  loadAddon: ReturnType<typeof vi.fn>
  reset: ReturnType<typeof vi.fn>
  write: ReturnType<typeof vi.fn>
  refresh: ReturnType<typeof vi.fn>
  unicode: { activeVersion: string }
}

function latestTerminal(): MockTerminalInstance | null {
  return (
    (
      globalThis as typeof globalThis & {
        __SENTINEL_TERMINAL_INSTANCES?: Array<MockTerminalInstance>
      }
    ).__SENTINEL_TERMINAL_INSTANCES?.at(-1) ?? null
  )
}

function terminalInstances(): Array<MockTerminalInstance> {
  return (
    (
      globalThis as typeof globalThis & {
        __SENTINEL_TERMINAL_INSTANCES?: Array<MockTerminalInstance>
      }
    ).__SENTINEL_TERMINAL_INSTANCES ?? []
  )
}

type TerminalHookProps = {
  openTabs: Array<string>
  activeSession: string
  activeEpoch: number
  sidebarCollapsed: boolean
  onAttachedMobile: () => void
}

function renderTerminalHook(overrides: Partial<TerminalHookProps> = {}) {
  let props: TerminalHookProps = {
    openTabs: overrides.openTabs ?? ['test-session'],
    activeSession: overrides.activeSession ?? 'test-session',
    activeEpoch: overrides.activeEpoch ?? 0,
    sidebarCollapsed: overrides.sidebarCollapsed ?? false,
    onAttachedMobile: overrides.onAttachedMobile ?? vi.fn(),
  }
  const hook = renderHook(
    (nextProps: TerminalHookProps) =>
      useTerminalTmux({
        openTabs: nextProps.openTabs,
        activeSession: nextProps.activeSession,
        activeEpoch: nextProps.activeEpoch,
        sidebarCollapsed: nextProps.sidebarCollapsed,
        onAttachedMobile: nextProps.onAttachedMobile,
      }),
    { initialProps: props },
  )
  return {
    ...hook,
    rerender(nextOverrides: Partial<TerminalHookProps> = {}) {
      props = { ...props, ...nextOverrides }
      hook.rerender(props)
    },
  }
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

describe('useTerminalTmux – shared input boundary', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('consumes sticky modifiers only after an accepted special-key send', () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()

    act(() => {
      result.current.toggleModifier('ctrl')
    })
    expect(result.current.modifiers.ctrl).toBe('sticky')

    let accepted = false
    act(() => {
      accepted = result.current.sendKey('c')
    })

    expect(accepted).toBe(true)
    expect(Array.from(socket.send.mock.calls[0][0] as Uint8Array)).toEqual([3])
    expect(result.current.modifiers.ctrl).toBe('off')
  })

  it('preserves sticky modifiers when the socket rejects input', () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()

    act(() => {
      result.current.toggleModifier('alt')
    })
    socket.readyState = MockWebSocket.CLOSED

    let accepted = true
    act(() => {
      accepted = result.current.sendKey('x')
    })

    expect(accepted).toBe(false)
    expect(socket.send).not.toHaveBeenCalled()
    expect(result.current.modifiers.alt).toBe('sticky')
  })

  it('routes xterm data through the same modifier transformation', () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()
    const terminal = latestTerminal()
    if (!terminal) throw new Error('terminal not created')

    act(() => {
      result.current.lockModifier('shift')
      terminal.emitData('a')
    })

    expect(new TextDecoder().decode(socket.send.mock.calls[0][0] as Uint8Array)).toBe('A')
    expect(result.current.modifiers.shift).toBe('locked')
  })

  it('sends a sticky Ctrl chord immediately from mobile composition input', async () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()
    const host = document.createElement('div')
    document.body.append(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })
    const textarea = latestTerminal()?.textarea
    if (!textarea) throw new Error('terminal textarea not created')
    socket.send.mockClear()
    const xtermInputHandler = vi.fn()
    textarea.addEventListener('input', xtermInputHandler)

    act(() => {
      result.current.toggleModifier('ctrl')
      textarea.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }))
      textarea.value = 'c'
      textarea.dispatchEvent(
        new InputEvent('input', {
          bubbles: true,
          composed: true,
          data: 'c',
          inputType: 'insertCompositionText',
        }),
      )
    })

    expect(Array.from(socket.send.mock.calls[0][0] as Uint8Array)).toEqual([3])
    expect(result.current.modifiers.ctrl).toBe('off')
    expect(textarea.value).toBe('')
    expect(xtermInputHandler).not.toHaveBeenCalled()
    host.remove()
  })

  it('leaves ordinary and multi-character IME input on xterm native path', async () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()
    const host = document.createElement('div')
    document.body.append(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })
    const textarea = latestTerminal()?.textarea
    if (!textarea) throw new Error('terminal textarea not created')
    socket.send.mockClear()

    textarea.value = 'text'
    act(() => {
      textarea.dispatchEvent(
        new InputEvent('input', {
          bubbles: true,
          composed: true,
          data: 'text',
          inputType: 'insertCompositionText',
        }),
      )
    })

    expect(socket.send).not.toHaveBeenCalled()
    expect(textarea.value).toBe('text')
    host.remove()
  })

  it('resets modifiers when the active connection closes', () => {
    const { result } = renderTerminalHook()
    const socket = connectSession()

    act(() => {
      result.current.lockModifier('ctrl')
      socket.emitClose()
    })

    expect(result.current.modifiers.ctrl).toBe('off')
  })
})

describe('useTerminalTmux – touch selection mode', () => {
  beforeEach(() => {
    setupEnvironment()
    ;(
      globalThis as typeof globalThis & {
        __SENTINEL_IS_MOBILE_LAYOUT?: boolean
      }
    ).__SENTINEL_IS_MOBILE_LAYOUT = true
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('enters selection, exposes a selected range and copies while disconnected', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.append(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })
    const terminal = latestTerminal()
    if (!terminal) throw new Error('terminal not created')

    act(() => {
      result.current.enterSelectionMode()
    })
    expect(result.current.selectionMode).toBe(true)

    act(() => {
      ;(
        globalThis as typeof globalThis & {
          __SENTINEL_TOUCH_SELECTION_CHANGE?: (hasSelection: boolean) => void
        }
      ).__SENTINEL_TOUCH_SELECTION_CHANGE?.(true)
    })
    expect(result.current.hasSelection).toBe(true)

    terminal.getSelection.mockReturnValue('selected output')
    latestWS().readyState = MockWebSocket.CLOSED
    await act(async () => {
      await expect(result.current.copySelection()).resolves.toBe(true)
    })

    const writeClipboard = (
      globalThis as typeof globalThis & {
        __SENTINEL_WRITE_CLIPBOARD_TEXT?: ReturnType<typeof vi.fn>
      }
    ).__SENTINEL_WRITE_CLIPBOARD_TEXT
    expect(writeClipboard).toHaveBeenCalledWith('selected output')
    expect(result.current.selectionMode).toBe(false)
    expect(result.current.hasSelection).toBe(false)
    host.remove()
  })

  it('clears selection state on explicit cancel', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.append(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
      result.current.enterSelectionMode()
    })
    act(() => {
      result.current.cancelSelection()
    })

    expect(result.current.selectionMode).toBe(false)
    expect(latestTerminal()?.clearSelection).toHaveBeenCalled()
    host.remove()
  })
})

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

  it('includes the fitted terminal size when reconnecting an opened terminal', async () => {
    const { result } = renderTerminalHook()
    const ws = connectSession()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    Object.defineProperty(ws, 'readyState', {
      value: WebSocket.CLOSED,
      configurable: true,
    })

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    const lastURL = new URL(latestWS().url)
    expect(lastURL.searchParams.get('session')).toBe('test-session')
    expect(lastURL.searchParams.get('cols')).toBe('80')
    expect(lastURL.searchParams.get('rows')).toBe('24')

    host.remove()
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

  it('refreshes the renderer when the window regains focus', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.refresh.mockClear()

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      window.dispatchEvent(new Event('focus'))
    })

    expect(terminal?.refresh).toHaveBeenCalledWith(0, 23)

    host.remove()
  })

  it('only refreshes the active session renderer when the window regains focus', async () => {
    const { result } = renderTerminalHook({
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

    const [activeTerminal, backgroundTerminal] = terminalInstances()
    activeTerminal?.refresh.mockClear()
    backgroundTerminal?.refresh.mockClear()

    act(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true,
      })
      window.dispatchEvent(new Event('focus'))
    })

    expect(activeTerminal?.refresh).toHaveBeenCalledWith(0, 23)
    expect(backgroundTerminal?.refresh).not.toHaveBeenCalled()

    hostA.remove()
    hostB.remove()
  })

  it('updates font size for all runtimes but only refits the active runtime', async () => {
    const { result } = renderTerminalHook({
      openTabs: ['session-a', 'session-b'],
      activeSession: 'session-a',
    })
    const hostA = document.createElement('div')
    const hostB = document.createElement('div')
    document.body.append(hostA, hostB)

    await act(async () => {
      result.current.getTerminalHostRef('session-a')(hostA)
      result.current.getTerminalHostRef('session-b')(hostB)
      await Promise.resolve()
    })

    const [activeTerminal, backgroundTerminal] = terminalInstances()
    activeTerminal.refresh.mockClear()
    backgroundTerminal.refresh.mockClear()

    const nextFontSize = Number(activeTerminal.options.fontSize) + 1

    act(() => {
      result.current.zoomIn()
    })

    expect(activeTerminal.options.fontSize).toBe(nextFontSize)
    expect(backgroundTerminal.options.fontSize).toBe(nextFontSize)
    expect(activeTerminal.refresh).toHaveBeenCalled()
    expect(backgroundTerminal.refresh).not.toHaveBeenCalled()

    hostA.remove()
    hostB.remove()
  })

  it('debounces clipboard writes from terminal selection changes', async () => {
    vi.useFakeTimers()
    try {
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText: vi.fn() },
        configurable: true,
      })
      const writeClipboard = vi.fn()
      ;(
        globalThis as typeof globalThis & {
          __SENTINEL_WRITE_CLIPBOARD_TEXT?: (text: string) => void
        }
      ).__SENTINEL_WRITE_CLIPBOARD_TEXT = writeClipboard
      const { result } = renderTerminalHook()
      const host = document.createElement('div')
      document.body.appendChild(host)

      await act(async () => {
        result.current.getTerminalHostRef('test-session')(host)
        await Promise.resolve()
      })

      const terminal = latestTerminal()
      terminal?.getSelection.mockReturnValue('selected text')
      const onSelection = terminal?.onSelectionChange.mock.calls[0]?.[0] as (() => void) | undefined

      act(() => {
        onSelection?.()
        onSelection?.()
        onSelection?.()
      })
      expect(writeClipboard).not.toHaveBeenCalled()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(119)
      })
      expect(writeClipboard).not.toHaveBeenCalled()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1)
      })
      expect(writeClipboard).toHaveBeenCalledTimes(1)
      expect(writeClipboard).toHaveBeenCalledWith('selected text')

      host.remove()
    } finally {
      Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
      vi.useRealTimers()
    }
  })

  it('flushes selection clipboard fallback on user selection end', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    const writeClipboard = vi.fn()
    ;(
      globalThis as typeof globalThis & {
        __SENTINEL_WRITE_CLIPBOARD_TEXT?: (text: string) => void
      }
    ).__SENTINEL_WRITE_CLIPBOARD_TEXT = writeClipboard
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const terminal = latestTerminal()
    terminal?.getSelection.mockReturnValue('fallback selection')
    const onSelection = terminal?.onSelectionChange.mock.calls[0]?.[0] as (() => void) | undefined

    act(() => {
      onSelection?.()
    })
    expect(writeClipboard).not.toHaveBeenCalled()

    fireEvent.mouseUp(document)

    expect(writeClipboard).toHaveBeenCalledTimes(1)
    expect(writeClipboard).toHaveBeenCalledWith('fallback selection')

    host.remove()
  })

  it('only refreshes the active session renderer on the periodic refresh', async () => {
    vi.useFakeTimers()
    try {
      const { result } = renderTerminalHook({
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

      const [activeTerminal, backgroundTerminal] = terminalInstances()
      activeTerminal?.refresh.mockClear()
      backgroundTerminal?.refresh.mockClear()

      act(() => {
        Object.defineProperty(document, 'visibilityState', {
          value: 'visible',
          writable: true,
          configurable: true,
        })
        vi.advanceTimersByTime(5 * 60 * 1000)
      })

      expect(activeTerminal?.refresh).toHaveBeenCalledWith(0, 23)
      expect(backgroundTerminal?.refresh).not.toHaveBeenCalled()

      hostA.remove()
      hostB.remove()
    } finally {
      vi.useRealTimers()
    }
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
      latestTerminal()?.refresh.mockClear()
      window.dispatchEvent(new CustomEvent('sentinel-theme-change', { detail: 'dracula' }))
      await Promise.resolve()
    })

    expect(host.style.backgroundColor).not.toBe(initialBackground)
    expect(terminalRoot?.style.backgroundColor).toBe(host.style.backgroundColor)
    expect(latestTerminal()?.refresh).toHaveBeenCalledTimes(1)

    host.remove()
  })

  it('does not load a WebGL renderer addon', () => {
    renderTerminalHook()
    expect(latestTerminal()?.loadAddon).toHaveBeenCalledTimes(6)
  })

  it('assigns a name to the hidden terminal textarea', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    expect(host.querySelector('textarea')?.getAttribute('name')).toBe('terminal-input-test-session')

    host.remove()
  })

  it('activates the unicode-graphemes provider so emoji widths match tmux', () => {
    renderTerminalHook()
    expect(latestTerminal()?.unicode.activeVersion).toBe('15-graphemes')
  })

  it('uses fallback fonts for powerline symbols and emoji before generic monospace', () => {
    renderTerminalHook()

    const fontFamily = latestTerminal()?.options.fontFamily
    expect(fontFamily).toEqual(expect.stringContaining('Symbols Nerd Font'))
    expect(fontFamily).toEqual(expect.stringContaining('Noto Color Emoji'))
    expect(fontFamily).toEqual(expect.stringMatching(/monospace$/))
  })

  it('enables glyph options that keep box drawing and ambiguous symbols aligned', () => {
    renderTerminalHook()

    expect(latestTerminal()?.options.customGlyphs).toBe(true)
    expect(latestTerminal()?.options.rescaleOverlappingGlyphs).toBe(true)
    expect(latestTerminal()?.options.minimumContrastRatio).toBe(4.5)
    expect(latestTerminal()?.options.smoothScrollDuration).toBe(0)
  })
})

describe('useTerminalTmux – resize traffic', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('sends the terminal size once per socket and skips unchanged fit repeats', async () => {
    const { result } = renderTerminalHook()
    const host = document.createElement('div')
    document.body.appendChild(host)

    await act(async () => {
      result.current.getTerminalHostRef('test-session')(host)
      await Promise.resolve()
    })

    const ws = connectSession()
    expect(ws.send).toHaveBeenCalledWith(JSON.stringify({ type: 'resize', cols: 80, rows: 24 }))

    const terminal = latestTerminal()
    ws.send.mockClear()
    terminal?.refresh.mockClear()
    act(() => {
      result.current.fitTerminal()
      result.current.fitTerminal()
    })

    expect(ws.send).not.toHaveBeenCalled()
    expect(terminal?.refresh).not.toHaveBeenCalled()

    if (terminal) {
      terminal.cols = 100
      terminal.rows = 30
    }
    act(() => {
      result.current.fitTerminal()
    })

    expect(ws.send).toHaveBeenCalledWith(JSON.stringify({ type: 'resize', cols: 100, rows: 30 }))
    expect(terminal?.refresh).toHaveBeenLastCalledWith(0, 29)

    host.remove()
  })
})

describe('useTerminalTmux – incoming terminal writes', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setupEnvironment()
  })

  afterEach(() => {
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
  })

  it('batches multiple websocket chunks into one xterm write per frame', () => {
    renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([65, 66]).buffer,
        }),
      )
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([67]).buffer,
        }),
      )
    })

    expect(terminal?.write).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).toHaveBeenCalledTimes(1)
    expect(Array.from(terminal?.write.mock.calls[0]?.[0] as Uint8Array)).toEqual([65, 66, 67])
  })

  it('exposes terminal write metrics for smoke validation', () => {
    const { unmount } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    const windowWithMetrics = window as typeof window & {
      __SENTINEL_TERMINAL_METRICS?: {
        renderer: 'dom'
        writeBatchCount: number
        writeBytes: number
        writeMaxQueueBytes: number
        writeRecoveries: number
      }
    }
    const metrics = windowWithMetrics.__SENTINEL_TERMINAL_METRICS
    terminal?.write.mockClear()

    expect(metrics).toMatchObject({
      renderer: 'dom',
      writeBatchCount: 0,
      writeBytes: 0,
      writeMaxQueueBytes: 0,
      writeRecoveries: 0,
    })

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([65, 66, 67]).buffer,
        }),
      )
    })
    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(metrics).toMatchObject({
      renderer: 'dom',
      writeBatchCount: 1,
      writeBytes: 3,
      writeMaxQueueBytes: 3,
      writeRecoveries: 0,
    })

    unmount()
    expect(windowWithMetrics.__SENTINEL_TERMINAL_METRICS).toBeUndefined()
  })

  it('uses the timeout fallback when animation frames are suspended', () => {
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(123)
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)

    try {
      renderTerminalHook()
      const terminal = latestTerminal()
      const ws = connectSession()
      terminal?.write.mockClear()

      act(() => {
        ws.onmessage?.(
          new MessageEvent('message', {
            data: new Uint8Array([65]).buffer,
          }),
        )
      })
      act(() => {
        vi.advanceTimersByTime(49)
      })
      expect(terminal?.write).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(1)
      })

      expect(requestFrame).toHaveBeenCalled()
      expect(cancelFrame).toHaveBeenCalledWith(123)
      expect(terminal?.write).toHaveBeenCalledTimes(1)
      expect(Array.from(terminal?.write.mock.calls[0]?.[0] as Uint8Array)).toEqual([65])
    } finally {
      requestFrame.mockRestore()
      cancelFrame.mockRestore()
    }
  })

  it('waits for xterm parser callbacks before draining oversized output', () => {
    renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()

    const firstChunk = new Uint8Array(700_000).fill(65)
    const secondChunk = new Uint8Array(700_000).fill(66)

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: firstChunk.buffer,
        }),
      )
      ws.onmessage?.(
        new MessageEvent('message', {
          data: secondChunk.buffer,
        }),
      )
    })

    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).toHaveBeenCalledTimes(1)
    const firstWrite = terminal?.write.mock.calls[0]?.[0] as Uint8Array | undefined
    expect(firstWrite?.byteLength).toBe(firstChunk.byteLength)
    expect(firstWrite?.[0]).toBe(65)

    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).toHaveBeenCalledTimes(1)

    const firstWriteDone = terminal?.write.mock.calls[0]?.[1] as (() => void) | undefined
    act(() => {
      firstWriteDone?.()
    })

    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).toHaveBeenCalledTimes(2)
    const secondWrite = terminal?.write.mock.calls[1]?.[0] as Uint8Array | undefined
    expect(secondWrite?.byteLength).toBe(secondChunk.byteLength)
    expect(secondWrite?.[0]).toBe(66)
  })

  it('drops queued output from a stale websocket generation on forced reconnect', () => {
    const { result } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([65]).buffer,
        }),
      )
    })

    act(() => {
      result.current.reconnectActiveSession({ force: true })
    })
    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).not.toHaveBeenCalled()
  })

  it('keeps a current-generation fallback flush after a stale frame callback', () => {
    const { result } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()

    const rafCallbacks: Array<FrameRequestCallback> = []
    let nextRafId = 100
    const requestFrame = vi
      .spyOn(window, 'requestAnimationFrame')
      .mockImplementation((callback) => {
        rafCallbacks.push(callback)
        nextRafId += 1
        return nextRafId
      })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)

    try {
      act(() => {
        ws.onmessage?.(
          new MessageEvent('message', {
            data: new Uint8Array([65]).buffer,
          }),
        )
      })
      expect(rafCallbacks).toHaveLength(1)

      act(() => {
        result.current.reconnectActiveSession({ force: true })
      })
      const nextWS = latestWS()
      act(() => {
        nextWS.emitOpen()
        nextWS.onmessage?.(
          new MessageEvent('message', {
            data: new Uint8Array([66]).buffer,
          }),
        )
      })
      expect(rafCallbacks).toHaveLength(2)

      act(() => {
        rafCallbacks[0]?.(0)
      })
      act(() => {
        vi.advanceTimersByTime(50)
      })

      expect(terminal?.write).toHaveBeenCalledTimes(1)
      expect(Array.from(terminal?.write.mock.calls[0]?.[0] as Uint8Array)).toEqual([66])
    } finally {
      requestFrame.mockRestore()
      cancelFrame.mockRestore()
    }
  })

  it('clears pending output before resetting the active terminal', () => {
    const { result } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()
    terminal?.reset.mockClear()

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([65]).buffer,
        }),
      )
    })

    act(() => {
      result.current.resetTerminal()
    })
    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.reset).toHaveBeenCalledTimes(1)
    expect(terminal?.write).not.toHaveBeenCalled()
  })

  it('recovers when xterm never reports that a write finished parsing', () => {
    const { result } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()
    terminal?.reset.mockClear()
    const countBefore = MockWebSocket.instances.length

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: new Uint8Array([65]).buffer,
        }),
      )
    })
    act(() => {
      vi.advanceTimersByTime(16)
    })

    expect(terminal?.write).toHaveBeenCalledTimes(1)
    expect(terminal?.reset).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(4_999)
    })
    expect(terminal?.reset).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(1)
    })

    expect(terminal?.reset).toHaveBeenCalledTimes(1)
    expect(result.current.connectionState).toBe('connecting')
    expect(result.current.statusDetail).toBe('reconnecting in 2s')

    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    expect(MockWebSocket.instances).toHaveLength(countBefore + 1)
  })

  it('reconnects instead of letting the incoming write queue grow without bound', () => {
    const { result } = renderTerminalHook()
    const terminal = latestTerminal()
    const ws = connectSession()
    terminal?.write.mockClear()
    terminal?.reset.mockClear()
    const countBefore = MockWebSocket.instances.length

    const chunk = new Uint8Array(1_048_576)
    act(() => {
      for (let i = 0; i < 17; i += 1) {
        ws.onmessage?.(
          new MessageEvent('message', {
            data: chunk.buffer,
          }),
        )
      }
    })

    expect(terminal?.write).not.toHaveBeenCalled()
    expect(terminal?.reset).toHaveBeenCalledTimes(1)
    expect(result.current.connectionState).toBe('connecting')
    expect(result.current.statusDetail).toBe('reconnecting in 2s')

    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    expect(MockWebSocket.instances).toHaveLength(countBefore + 1)
  })
})

describe('useTerminalTmux – renderer refresh', () => {
  beforeEach(() => {
    setupEnvironment()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
  })

  it('refreshes the newly active session renderer when switching tabs', async () => {
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
    activeNextTerminal?.refresh.mockClear()

    act(() => {
      rerender({
        openTabs: ['session-a', 'session-b'],
        activeSession: 'session-b',
        activeEpoch: 1,
      })
    })

    expect(activeNextTerminal?.refresh).toHaveBeenCalledWith(0, 23)

    hostA.remove()
    hostB.remove()
  })
})
