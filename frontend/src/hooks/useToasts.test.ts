// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useToasts } from './useToasts'

describe('useToasts', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts with no toasts', () => {
    const { result } = renderHook(() => useToasts())
    expect(result.current.toasts).toEqual([])
  })

  it('pushToast adds a toast', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({
        level: 'success',
        title: 'Done',
        message: 'It worked',
      })
    })

    expect(result.current.toasts).toHaveLength(1)
    expect(result.current.toasts[0].level).toBe('success')
    expect(result.current.toasts[0].title).toBe('Done')
    expect(result.current.toasts[0].message).toBe('It worked')
  })

  it('dismissToast removes a toast by id', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({
        level: 'info',
        title: 'A',
        message: 'first',
      })
      result.current.pushToast({
        level: 'info',
        title: 'B',
        message: 'second',
      })
    })

    const idToRemove = result.current.toasts[0].id

    act(() => {
      result.current.dismissToast(idToRemove)
    })

    expect(result.current.toasts).toHaveLength(1)
    expect(result.current.toasts[0].title).toBe('B')
  })

  it('auto-dismisses after default TTL', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({
        level: 'info',
        title: 'Temp',
        message: 'goes away',
      })
    })

    expect(result.current.toasts).toHaveLength(1)

    act(() => {
      vi.advanceTimersByTime(3600)
    })

    expect(result.current.toasts).toHaveLength(0)
  })

  it('auto-dismisses after custom TTL', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({
        level: 'error',
        title: 'Quick',
        message: 'fast',
        ttlMs: 1000,
      })
    })

    expect(result.current.toasts).toHaveLength(1)

    act(() => {
      vi.advanceTimersByTime(999)
    })
    expect(result.current.toasts).toHaveLength(1)

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(result.current.toasts).toHaveLength(0)
  })

  it('caps at 5 toasts, dropping oldest', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      for (let i = 1; i <= 7; i++) {
        result.current.pushToast({
          level: 'info',
          title: `T${i}`,
          message: `msg${i}`,
          ttlMs: 60_000,
        })
      }
    })

    expect(result.current.toasts).toHaveLength(5)
    expect(result.current.toasts[0].title).toBe('T3')
    expect(result.current.toasts[4].title).toBe('T7')
  })

  it('assigns unique incremental ids', () => {
    const { result } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({ level: 'info', title: 'A', message: 'a' })
      result.current.pushToast({ level: 'info', title: 'B', message: 'b' })
    })

    const ids = result.current.toasts.map((t) => t.id)
    expect(ids[0]).toBeLessThan(ids[1])
    expect(new Set(ids).size).toBe(2)
  })

  it('clears timers on unmount', () => {
    const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout')

    const { result, unmount } = renderHook(() => useToasts())

    act(() => {
      result.current.pushToast({
        level: 'info',
        title: 'A',
        message: 'a',
        ttlMs: 60_000,
      })
    })

    unmount()

    expect(clearTimeoutSpy).toHaveBeenCalled()
    clearTimeoutSpy.mockRestore()
  })
})
