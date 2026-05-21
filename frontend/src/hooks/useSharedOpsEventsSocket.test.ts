// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSharedOpsEventsSocket } from './useSharedOpsEventsSocket'

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
}

describe('useSharedOpsEventsSocket', () => {
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

  it('force reconnect opens a fresh shared socket for active subscribers', () => {
    const onMessage = vi.fn()
    const timeoutSpy = vi.spyOn(window, 'setTimeout')
    const { result } = renderHook(() =>
      useSharedOpsEventsSocket({
        authenticated: true,
        tokenRequired: false,
      }),
    )

    let unsubscribe = () => {}
    act(() => {
      unsubscribe = result.current.subscribe(onMessage)
    })
    expect(MockWebSocket.instances).toHaveLength(1)

    act(() => {
      MockWebSocket.instances[0].emitOpen()
    })
    expect(result.current.connectionState).toBe('connected')

    act(() => {
      result.current.forceReconnect()
    })

    expect(MockWebSocket.instances).toHaveLength(2)
    expect(timeoutSpy).not.toHaveBeenCalled()
    expect(result.current.connectionState).toBe('connecting')

    act(() => {
      unsubscribe()
    })
  })
})
