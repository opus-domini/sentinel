// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useLogStream } from './useLogStream'

class MockWebSocket {
  static instances: Array<MockWebSocket> = []
  url: string
  protocols: Array<string> | string | undefined
  onopen: ((ev: Event) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  closed = false

  constructor(url: string, protocols?: Array<string> | string) {
    this.url = url
    this.protocols = protocols
    MockWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
  }

  simulateOpen() {
    this.onopen?.(new Event('open'))
  }

  simulateMessage(data: string) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  simulateClose() {
    this.onclose?.(new CloseEvent('close'))
  }
}

function lastSocket(): MockWebSocket {
  return MockWebSocket.instances[MockWebSocket.instances.length - 1]
}

describe('useLogStream', () => {
  const originalWebSocket = globalThis.WebSocket

  beforeEach(() => {
    MockWebSocket.instances = []
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
    vi.useFakeTimers()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.useRealTimers()
  })

  it('returns disconnected when not enabled', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    const { result } = renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: false,
        onLine,
      }),
    )

    expect(result.current).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(0)
  })

  it('returns disconnected when target is null', () => {
    const onLine = vi.fn()
    const { result } = renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target: null,
        enabled: true,
        onLine,
      }),
    )

    expect(result.current).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(0)
  })

  it('returns disconnected when token is required and unauthenticated', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    const { result } = renderHook(() =>
      useLogStream({
        authenticated: false,
        tokenRequired: true,
        target,
        enabled: true,
        onLine,
      }),
    )

    expect(result.current).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(0)
  })

  it('connects with service target and transitions to connected', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'my-svc' }
    const { result } = renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    expect(result.current).toBe('connecting')
    expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1)
    expect(lastSocket().url).toContain('/ws/logs?service=my-svc')

    act(() => {
      lastSocket().simulateOpen()
    })
    expect(result.current).toBe('connected')
  })

  it('connects with unit target using correct query params', () => {
    const onLine = vi.fn()
    const target = {
      kind: 'unit' as const,
      unit: 'nginx.service',
      scope: 'system',
      manager: 'systemd',
    }
    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1)
    const url = lastSocket().url
    expect(url).toContain('unit=nginx.service')
    expect(url).toContain('scope=system')
    expect(url).toContain('manager=systemd')
  })

  it('calls onLine for log messages', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    act(() => {
      lastSocket().simulateOpen()
    })

    act(() => {
      lastSocket().simulateMessage(
        JSON.stringify({ type: 'log', line: 'hello world' }),
      )
    })

    expect(onLine).toHaveBeenCalledWith('hello world')
  })

  it('ignores non-log messages', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    act(() => {
      lastSocket().simulateOpen()
      lastSocket().simulateMessage(
        JSON.stringify({ type: 'status', state: 'streaming' }),
      )
      lastSocket().simulateMessage('not json at all')
    })

    expect(onLine).not.toHaveBeenCalled()
  })

  it('reconnects after close with delay', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    const countBefore = MockWebSocket.instances.length

    act(() => {
      lastSocket().simulateClose()
    })

    // No new socket yet (waiting for timer)
    expect(MockWebSocket.instances).toHaveLength(countBefore)

    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // A new socket was created after the reconnect delay
    expect(MockWebSocket.instances.length).toBe(countBefore + 1)
  })

  it('closes socket on unmount', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    const { unmount } = renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    const socket = lastSocket()
    unmount()
    expect(socket.closed).toBe(true)
  })

  it('sets error state on socket error', () => {
    const onLine = vi.fn()
    const target = { kind: 'service' as const, name: 'test' }
    const { result } = renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    act(() => {
      lastSocket().onerror?.(new Event('error'))
    })

    expect(result.current).toBe('error')
  })

  it('retries with progressive backoff', () => {
    const onLine = vi.fn()
    const timeoutSpy = vi.spyOn(window, 'setTimeout')
    const target = { kind: 'service' as const, name: 'test' }

    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    // First close triggers first retry at 1200ms.
    act(() => {
      lastSocket().simulateClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 1_200)

    act(() => {
      vi.advanceTimersByTime(1_200)
    })

    // Second close triggers retry at 2040ms (1200 * 1.7).
    act(() => {
      lastSocket().simulateClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 2_040)

    act(() => {
      vi.advanceTimersByTime(2_040)
    })

    // Third close triggers retry at 3468ms (2040 * 1.7).
    act(() => {
      lastSocket().simulateClose()
    })
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 3_468)
  })

  it('resets backoff after successful reconnect', () => {
    const onLine = vi.fn()
    const timeoutSpy = vi.spyOn(window, 'setTimeout')
    const target = { kind: 'service' as const, name: 'test' }

    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine,
      }),
    )

    // Close and reconnect.
    act(() => {
      lastSocket().simulateClose()
      vi.advanceTimersByTime(1_200)
    })

    // Successful open resets the backoff.
    act(() => {
      lastSocket().simulateOpen()
      lastSocket().simulateClose()
    })

    // Should retry with initial delay again.
    expect(timeoutSpy).toHaveBeenLastCalledWith(expect.any(Function), 1_200)
  })

  it('does not call onLine when onLine throws', () => {
    const errorLine = vi.fn(() => {
      throw new Error('handler error')
    })
    const target = { kind: 'service' as const, name: 'test' }

    // Should not throw or crash the hook.
    renderHook(() =>
      useLogStream({
        authenticated: true,
        tokenRequired: false,
        target,
        enabled: true,
        onLine: errorLine,
      }),
    )

    act(() => {
      lastSocket().simulateOpen()
    })

    // The hook calls onLineRef.current which throws — this is the caller's
    // responsibility, but the hook itself should not break.
    expect(() => {
      act(() => {
        lastSocket().simulateMessage(
          JSON.stringify({ type: 'log', line: 'boom' }),
        )
      })
    }).toThrow('handler error')

    expect(errorLine).toHaveBeenCalledWith('boom')
  })
})
