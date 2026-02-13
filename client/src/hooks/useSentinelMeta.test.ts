// @vitest-environment jsdom
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSentinelMeta } from './useSentinelMeta'

describe('useSentinelMeta', () => {
  const originalFetch = globalThis.fetch

  beforeEach(() => {
    globalThis.fetch = vi.fn()
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
  })

  function mockFetch(status: number, body: unknown): void {
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: status >= 200 && status < 300,
      status,
      json: () => Promise.resolve(body),
    })
  }

  it('starts with default values', () => {
    mockFetch(200, { data: { tokenRequired: false } })
    const { result } = renderHook(() => useSentinelMeta(''))
    expect(result.current.tokenRequired).toBe(false)
    expect(result.current.defaultCwd).toBe('')
    expect(result.current.unauthorized).toBe(false)
  })

  it('sets tokenRequired from API response', async () => {
    mockFetch(200, { data: { tokenRequired: true } })

    const { result } = renderHook(() => useSentinelMeta(''))

    await waitFor(() => {
      expect(result.current.tokenRequired).toBe(true)
    })
    expect(result.current.unauthorized).toBe(false)
  })

  it('sets defaultCwd from API response', async () => {
    mockFetch(200, { data: { tokenRequired: false, defaultCwd: '/home/hugo' } })

    const { result } = renderHook(() => useSentinelMeta(''))

    await waitFor(() => {
      expect(result.current.defaultCwd).toBe('/home/hugo')
    })
  })

  it('sets unauthorized on 401', async () => {
    mockFetch(401, {})

    const { result } = renderHook(() => useSentinelMeta('bad-token'))

    await waitFor(() => {
      expect(result.current.unauthorized).toBe(true)
    })
    expect(result.current.tokenRequired).toBe(true)
    expect(result.current.defaultCwd).toBe('')
  })

  it('sends bearer token in request', async () => {
    mockFetch(200, { data: { tokenRequired: true } })

    renderHook(() => useSentinelMeta('my-token'))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalled()
    })

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBe('Bearer my-token')
  })

  it('does not send auth header for empty token', async () => {
    mockFetch(200, { data: { tokenRequired: false } })

    renderHook(() => useSentinelMeta(''))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalled()
    })

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBeUndefined()
  })

  it('keeps defaults on non-ok non-401 response', async () => {
    mockFetch(500, {})

    const { result } = renderHook(() => useSentinelMeta(''))

    // Wait for fetch to complete.
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalled()
    })

    // Small delay to ensure state would have been set if it were going to.
    await new Promise((r) => setTimeout(r, 10))

    expect(result.current.tokenRequired).toBe(false)
    expect(result.current.defaultCwd).toBe('')
    expect(result.current.unauthorized).toBe(false)
  })

  it('keeps defaults on network error', async () => {
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error('network fail'),
    )

    const { result } = renderHook(() => useSentinelMeta(''))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalled()
    })

    await new Promise((r) => setTimeout(r, 10))

    expect(result.current.tokenRequired).toBe(false)
    expect(result.current.defaultCwd).toBe('')
    expect(result.current.unauthorized).toBe(false)
  })
})
