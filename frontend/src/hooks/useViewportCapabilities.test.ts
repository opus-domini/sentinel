// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useViewportCapabilities } from './useViewportCapabilities'

type QueryState = Record<string, boolean>

describe('useViewportCapabilities', () => {
  let queryState: QueryState
  let listeners: Map<string, Set<() => void>>

  beforeEach(() => {
    queryState = {}
    listeners = new Map()
    Object.defineProperty(navigator, 'maxTouchPoints', {
      configurable: true,
      value: 0,
    })
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation((query: string) => ({
        media: query,
        get matches() {
          return queryState[query] ?? false
        },
        addEventListener: (_: string, listener: () => void) => {
          const queryListeners = listeners.get(query) ?? new Set()
          queryListeners.add(listener)
          listeners.set(query, queryListeners)
        },
        removeEventListener: (_: string, listener: () => void) => {
          listeners.get(query)?.delete(listener)
        },
      })),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function updateQuery(query: string, matches: boolean) {
    queryState[query] = matches
    act(() => {
      for (const listener of listeners.get(query) ?? []) {
        listener()
      }
    })
  }

  it('keeps a non-touch desktop outside compact and touch policies', () => {
    const { result } = renderHook(() => useViewportCapabilities())
    expect(result.current).toEqual({
      compactLayout: false,
      touchCapable: false,
      touchOptimized: false,
    })
  })

  it('uses compact and touch-optimized layout for portrait and phone landscape footprints', () => {
    const compactQuery = '(max-width: 767px), (max-width: 1024px) and (max-height: 500px)'
    queryState[compactQuery] = true

    const { result } = renderHook(() => useViewportCapabilities())
    expect(result.current).toEqual({
      compactLayout: true,
      touchCapable: false,
      touchOptimized: true,
    })

    updateQuery(compactQuery, false)
    expect(result.current.compactLayout).toBe(false)
    expect(result.current.touchOptimized).toBe(false)
  })

  it('keeps a wide hybrid device touch-safe without changing its desktop structure', () => {
    Object.defineProperty(navigator, 'maxTouchPoints', {
      configurable: true,
      value: 5,
    })

    const { result } = renderHook(() => useViewportCapabilities())
    expect(result.current).toEqual({
      compactLayout: false,
      touchCapable: true,
      touchOptimized: true,
    })
  })

  it('reacts to coarse pointer changes and removes every listener on unmount', () => {
    const { result, unmount } = renderHook(() => useViewportCapabilities())

    updateQuery('(any-pointer: coarse)', true)
    expect(result.current.touchCapable).toBe(true)
    expect(result.current.touchOptimized).toBe(true)

    unmount()
    expect(
      Array.from(listeners.values()).every((queryListeners) => queryListeners.size === 0),
    ).toBe(true)
  })
})
