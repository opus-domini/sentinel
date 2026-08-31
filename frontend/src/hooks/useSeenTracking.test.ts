// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiFunction, PresenceSocketRef } from './tmuxTypes'
import { useSeenTracking } from './useSeenTracking'

const NO_PATCHES = {
  hasInputPatches: false,
  applied: false,
  hasUnknownSession: false,
}

type SocketStub = {
  readyState: number
  send: ReturnType<typeof vi.fn>
}

function openSocket(): SocketStub {
  return { readyState: WebSocket.OPEN, send: vi.fn() }
}

type Options = Parameters<typeof useSeenTracking>[0]

function makeOptions(overrides?: Partial<Options>): Options {
  return {
    api: vi.fn(() => Promise.resolve({ acked: true })) as unknown as ApiFunction,
    presenceSocketRef: { current: null } as PresenceSocketRef,
    activeSession: 'main',
    activeWindowIndex: null,
    activePaneID: '%1',
    applySessionActivityPatches: vi.fn(() => NO_PATCHES),
    applyInspectorProjectionPatches: vi.fn(() => false),
    ...overrides,
  }
}

function sentRequestID(socket: SocketStub, callIndex = 0): string {
  const raw = socket.send.mock.calls[callIndex][0] as string
  return (JSON.parse(raw) as { requestId: string }).requestId
}

describe('useSeenTracking', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('marks the active pane over the websocket and skips the HTTP fallback on ack', async () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    const { result } = renderHook(() => useSeenTracking(opts))

    expect(socket.send).toHaveBeenCalledTimes(1)
    const payload = JSON.parse(socket.send.mock.calls[0][0] as string) as Record<string, unknown>
    expect(payload.type).toBe('seen')
    expect(payload.session).toBe('main')
    expect(payload.scope).toBe('pane')
    expect(payload.paneId).toBe('%1')

    const requestId = sentRequestID(socket)
    await act(async () => {
      result.current.seenAckWaitersRef.current.get(requestId)?.(true)
      await Promise.resolve()
    })

    expect(opts.api).not.toHaveBeenCalled()
    expect(result.current.seenAckWaitersRef.current.size).toBe(0)
  })

  it('falls back to HTTP exactly once when the ack misses its 800ms deadline', async () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    renderHook(() => useSeenTracking(opts))

    expect(opts.api).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(799)
      await Promise.resolve()
    })
    expect(opts.api).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(1)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(opts.api).toHaveBeenCalledTimes(1)
    const [path, init] = (opts.api as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(path).toBe('/api/tmux/sessions/main/seen')
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      scope: 'pane',
      paneId: '%1',
    })

    // The late ack must not produce a second mark.
    await act(async () => {
      vi.advanceTimersByTime(5_000)
      await Promise.resolve()
    })
    expect(opts.api).toHaveBeenCalledTimes(1)
  })

  it('drains and clears every pending ack waiter when the socket drops', async () => {
    const socket = openSocket()
    const opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    const { result } = renderHook(() => useSeenTracking(opts))

    expect(result.current.seenAckWaitersRef.current.size).toBe(1)

    await act(async () => {
      result.current.settlePendingSeenAcks(false)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(result.current.seenAckWaitersRef.current.size).toBe(0)
    expect(opts.api).toHaveBeenCalledTimes(1)
  })

  it('emits exactly one mark per selection change and none for an unchanged one', () => {
    const socket = openSocket()
    let opts = makeOptions({
      presenceSocketRef: { current: socket } as unknown as PresenceSocketRef,
    })
    const { rerender } = renderHook(() => useSeenTracking(opts))

    expect(socket.send).toHaveBeenCalledTimes(1)

    rerender()
    expect(socket.send).toHaveBeenCalledTimes(1)

    opts = { ...opts, activePaneID: '%2' }
    rerender()
    expect(socket.send).toHaveBeenCalledTimes(2)
    expect((JSON.parse(socket.send.mock.calls[1][0] as string) as { paneId: string }).paneId).toBe(
      '%2',
    )

    opts = { ...opts, activePaneID: null, activeWindowIndex: 3 }
    rerender()
    expect(socket.send).toHaveBeenCalledTimes(3)
    const windowPayload = JSON.parse(socket.send.mock.calls[2][0] as string) as {
      scope: string
      windowIndex: number
    }
    expect(windowPayload.scope).toBe('window')
    expect(windowPayload.windowIndex).toBe(3)
  })

  it('goes straight to HTTP when no presence socket is open', async () => {
    const opts = makeOptions()
    renderHook(() => useSeenTracking(opts))

    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(opts.api).toHaveBeenCalledTimes(1)
    expect(opts.applySessionActivityPatches).toHaveBeenCalled()
    expect(opts.applyInspectorProjectionPatches).toHaveBeenCalled()
  })
})
