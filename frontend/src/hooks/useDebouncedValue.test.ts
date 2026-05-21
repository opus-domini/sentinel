// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useDebouncedValue } from './useDebouncedValue'

describe('useDebouncedValue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns the current value immediately on first render', () => {
    const { result } = renderHook(() => useDebouncedValue('alpha', 200))

    expect(result.current).toBe('alpha')
  })

  it('debounces non-empty updates', () => {
    let value = 'alpha'
    const { result, rerender } = renderHook(() => useDebouncedValue(value, 200))

    value = 'beta'
    rerender()

    expect(result.current).toBe('alpha')

    act(() => {
      vi.advanceTimersByTime(199)
    })
    expect(result.current).toBe('alpha')

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(result.current).toBe('beta')
  })

  it('cancels stale updates when the value changes again inside the window', () => {
    let value = 'a'
    const { result, rerender } = renderHook(() => useDebouncedValue(value, 200))

    value = 'ab'
    rerender()

    act(() => {
      vi.advanceTimersByTime(120)
    })

    value = 'abc'
    rerender()

    act(() => {
      vi.advanceTimersByTime(199)
    })
    expect(result.current).toBe('a')

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(result.current).toBe('abc')
  })

  it('flushes empty strings immediately', () => {
    let value = 'alpha'
    const { result, rerender } = renderHook(() => useDebouncedValue(value, 200))

    value = ''
    rerender()

    expect(result.current).toBe('')
  })
})
