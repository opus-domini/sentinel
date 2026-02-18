// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useOpsEventsSocket } from './useOpsEventsSocket'

class MockWebSocket {
  static instances: Array<MockWebSocket> = []

  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor(
    public url: string,
    public protocols?: string | Array<string>,
  ) {
    MockWebSocket.instances.push(this)
  }

  close() {
    this.onclose?.(new CloseEvent('close'))
  }

  emitOpen() {
    this.onopen?.(new Event('open'))
  }

  emitClose() {
    this.onclose?.(new CloseEvent('close'))
  }
}

describe('useOpsEventsSocket', () => {
  const originalWebSocket = globalThis.WebSocket

  beforeEach(() => {
    vi.useFakeTimers()
    MockWebSocket.instances = []
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
  })

  it('does not connect when token is required and missing', () => {
    const onMessage = vi.fn()
    const { result } = renderHook(() =>
      useOpsEventsSocket({
        token: '   ',
        tokenRequired: true,
        onMessage,
      }),
    )

    expect(result.current).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(0)
  })

  it('retries with progressive backoff', () => {
    const onMessage = vi.fn()
    const timeoutSpy = vi.spyOn(window, 'setTimeout')

    renderHook(() =>
      useOpsEventsSocket({
        token: '',
        tokenRequired: false,
        onMessage,
      }),
    )

    expect(MockWebSocket.instances).toHaveLength(1)

    act(() => {
      MockWebSocket.instances[0].emitClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 1_200)

    act(() => {
      vi.advanceTimersByTime(1_200)
    })
    expect(MockWebSocket.instances).toHaveLength(2)

    act(() => {
      MockWebSocket.instances[1].emitClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 2_040)

    act(() => {
      vi.advanceTimersByTime(2_040)
    })
    expect(MockWebSocket.instances).toHaveLength(3)

    act(() => {
      MockWebSocket.instances[2].emitClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 3_468)
  })

  it('resets retry delay after a successful reconnect', () => {
    const onMessage = vi.fn()
    const timeoutSpy = vi.spyOn(window, 'setTimeout')

    renderHook(() =>
      useOpsEventsSocket({
        token: '',
        tokenRequired: false,
        onMessage,
      }),
    )

    act(() => {
      MockWebSocket.instances[0].emitClose()
      vi.advanceTimersByTime(1_200)
    })

    expect(MockWebSocket.instances).toHaveLength(2)

    act(() => {
      MockWebSocket.instances[1].emitOpen()
      MockWebSocket.instances[1].emitClose()
    })

    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 1_200)
  })

  it('sets error state on socket error', () => {
    const onMessage = vi.fn()
    const { result } = renderHook(() =>
      useOpsEventsSocket({
        token: '',
        tokenRequired: false,
        onMessage,
      }),
    )

    expect(MockWebSocket.instances).toHaveLength(1)

    act(() => {
      MockWebSocket.instances[0].onerror?.(new Event('error'))
    })

    expect(result.current).toBe('error')
  })

  it('reconnects with a new socket when token changes', () => {
    const onMessage = vi.fn()
    let token = 'token-a'

    const { rerender } = renderHook(() =>
      useOpsEventsSocket({
        token,
        tokenRequired: true,
        onMessage,
      }),
    )

    expect(MockWebSocket.instances).toHaveLength(1)

    token = 'token-b'
    rerender()

    // Old socket is closed and a new one is created.
    expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(2)
  })

  it('delivers parsed JSON messages to onMessage callback', () => {
    const onMessage = vi.fn()
    renderHook(() =>
      useOpsEventsSocket({
        token: '',
        tokenRequired: false,
        onMessage,
      }),
    )

    act(() => {
      MockWebSocket.instances[0].emitOpen()
    })

    act(() => {
      MockWebSocket.instances[0].onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'test', value: 42 }),
        }),
      )
    })

    expect(onMessage).toHaveBeenCalledWith({ type: 'test', value: 42 })
  })

  it('ignores invalid JSON messages', () => {
    const onMessage = vi.fn()
    renderHook(() =>
      useOpsEventsSocket({
        token: '',
        tokenRequired: false,
        onMessage,
      }),
    )

    act(() => {
      MockWebSocket.instances[0].emitOpen()
    })

    act(() => {
      MockWebSocket.instances[0].onmessage?.(
        new MessageEvent('message', { data: 'not json' }),
      )
    })

    expect(onMessage).not.toHaveBeenCalled()
  })
})
