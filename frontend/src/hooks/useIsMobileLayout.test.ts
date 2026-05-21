// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useIsMobileLayout } from './useIsMobileLayout'

describe('useIsMobileLayout', () => {
  let listeners: Array<(event: { matches: boolean }) => void>
  let currentMatches: boolean

  beforeEach(() => {
    listeners = []
    currentMatches = false

    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation(() => ({
        get matches() {
          return currentMatches
        },
        addEventListener: (
          _: string,
          cb: (event: { matches: boolean }) => void,
        ) => {
          listeners.push(cb)
        },
        removeEventListener: (
          _: string,
          cb: (event: { matches: boolean }) => void,
        ) => {
          listeners = listeners.filter((l) => l !== cb)
        },
      })),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns false for desktop viewport', () => {
    currentMatches = false
    const { result } = renderHook(() => useIsMobileLayout())
    expect(result.current).toBe(false)
  })

  it('returns true for mobile viewport', () => {
    currentMatches = true
    const { result } = renderHook(() => useIsMobileLayout())
    expect(result.current).toBe(true)
  })

  it('updates when media query changes', () => {
    currentMatches = false
    const { result } = renderHook(() => useIsMobileLayout())
    expect(result.current).toBe(false)

    act(() => {
      for (const listener of listeners) {
        listener({ matches: true })
      }
    })

    expect(result.current).toBe(true)
  })

  it('cleans up listener on unmount', () => {
    currentMatches = false
    const { unmount } = renderHook(() => useIsMobileLayout())
    expect(listeners).toHaveLength(1)

    unmount()
    expect(listeners).toHaveLength(0)
  })
})
