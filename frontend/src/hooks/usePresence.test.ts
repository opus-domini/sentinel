// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiFunction, PresenceSocketRef, TabsStateRef } from './tmuxTypes'
import { usePresence } from './usePresence'

type SocketStub = {
  readyState: number
  send: ReturnType<typeof vi.fn>
}

function openSocket(): SocketStub {
  return { readyState: WebSocket.OPEN, send: vi.fn() }
}

type Options = Parameters<typeof usePresence>[0]

function makeOptions(overrides?: Partial<Options>): Options {
  return {
    api: vi.fn(() => Promise.resolve({ accepted: true })) as unknown as ApiFunction,
    presenceSocketRef: { current: null } as PresenceSocketRef,
    tabsStateRef: {
      current: { openTabs: ['main'], activeSession: 'main', activeEpoch: 0 },
    } as TabsStateRef,
    activeWindowIndex: 0,
    activePaneID: '%1',
    activeSession: 'main',
    ...overrides,
  }
}

describe('usePresence', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    window.sessionStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    window.sessionStorage.clear()
  })

  it('emits the current selection over the websocket and skips the HTTP fallback', () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    renderHook(() => usePresence(opts))

    expect(socket.send).toHaveBeenCalled()
    const payload = JSON.parse(socket.send.mock.calls[0][0] as string) as Record<string, unknown>
    expect(payload.type).toBe('presence')
    expect(payload.session).toBe('main')
    expect(payload.windowIndex).toBe(0)
    expect(payload.paneId).toBe('%1')
    expect(typeof payload.terminalId).toBe('string')
    expect(payload.terminalId).not.toBe('')
    expect(opts.api).not.toHaveBeenCalled()
  })

  it('throttles an unchanged heartbeat to one emit per 10s window', () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    renderHook(() => usePresence(opts))

    const initial = socket.send.mock.calls.length
    expect(initial).toBeGreaterThan(0)

    // The heartbeat ticks at 10s, and the signature is unchanged; the throttle
    // window opens at exactly the same interval, so one more emit is expected.
    act(() => {
      vi.advanceTimersByTime(9_999)
    })
    expect(socket.send).toHaveBeenCalledTimes(initial)

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(socket.send).toHaveBeenCalledTimes(initial + 1)
  })

  it('bypasses the throttle for a forced emit', () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    const { result } = renderHook(() => usePresence(opts))

    const initial = socket.send.mock.calls.length

    act(() => {
      expect(result.current.sendPresenceOverWS(false)).toBe(true)
    })
    expect(socket.send).toHaveBeenCalledTimes(initial)

    act(() => {
      expect(result.current.sendPresenceOverWS(true)).toBe(true)
    })
    expect(socket.send).toHaveBeenCalledTimes(initial + 1)
  })

  it('re-emits when the active pane changes even inside the throttle window', () => {
    const socket = openSocket()
    let opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    const { rerender } = renderHook(() => usePresence(opts))

    const initial = socket.send.mock.calls.length

    opts = { ...opts, activePaneID: '%2' }
    rerender()

    expect(socket.send).toHaveBeenCalledTimes(initial + 1)
    const payload = JSON.parse(socket.send.mock.calls[initial][0] as string) as { paneId: string }
    expect(payload.paneId).toBe('%2')
  })

  it('falls back to HTTP when no socket is open and never overlaps requests', async () => {
    const opts = makeOptions()
    renderHook(() => usePresence(opts))

    await act(async () => {
      await Promise.resolve()
    })

    expect(opts.api).toHaveBeenCalledTimes(1)
    const [path, init] = (opts.api as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(path).toBe('/api/tmux/presence')
    expect((init as RequestInit).method).toBe('PUT')

    // Two forced signals back to back: the second lands while the first request
    // is still in flight and must not start an overlapping one.
    act(() => {
      window.dispatchEvent(new Event('focus'))
      window.dispatchEvent(new Event('blur'))
    })
    expect(opts.api).toHaveBeenCalledTimes(2)
  })

  it('reuses the terminal id stored for the browsing session', () => {
    window.sessionStorage.setItem('sentinel.tmux.presence.terminalId', 'terminal-fixed')
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    renderHook(() => usePresence(opts))

    const payload = JSON.parse(socket.send.mock.calls[0][0] as string) as { terminalId: string }
    expect(payload.terminalId).toBe('terminal-fixed')
  })
})
