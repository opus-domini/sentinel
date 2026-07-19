// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useConnectionCheck } from './useConnectionCheck'

describe('useConnectionCheck', () => {
  const originalFetch = globalThis.fetch

  beforeEach(() => {
    globalThis.fetch = vi.fn()
  })

  afterEach(() => {
    cleanup()
    globalThis.fetch = originalFetch
  })

  it('marks the connection ready after a successful preflight', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(new Response(null, { status: 200 }))

    const { result } = renderHook(() =>
      useConnectionCheck({ enabled: true, onUnauthorized: vi.fn() }),
    )

    await waitFor(() => expect(result.current.ready).toBe(true))
    expect(result.current.issue).toBeNull()
    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/connection/check',
      expect.objectContaining({ method: 'POST', credentials: 'same-origin' }),
    )
  })

  it('exposes the exact proxy diagnosis and configuration', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            code: 'UNTRUSTED_PROXY',
            message: 'HTTPS proxy "192.0.2.10" is not trusted; add it to server.trusted_proxies',
            details: {
              configPath: '/root/.sentinel/config.toml',
              configuration: '[server]\ntrusted_proxies = ["192.0.2.10"]',
            },
          },
        }),
        { status: 403, headers: { 'Content-Type': 'application/json' } },
      ),
    )

    const { result } = renderHook(() =>
      useConnectionCheck({ enabled: true, onUnauthorized: vi.fn() }),
    )

    await waitFor(() => expect(result.current.issue?.code).toBe('UNTRUSTED_PROXY'))
    expect(result.current.ready).toBe(false)
    expect(result.current.issue).toEqual({
      code: 'UNTRUSTED_PROXY',
      title: 'HTTPS proxy is not trusted',
      message: 'HTTPS proxy "192.0.2.10" is not trusted; add it to server.trusted_proxies',
      configPath: '/root/.sentinel/config.toml',
      configuration: '[server]\ntrusted_proxies = ["192.0.2.10"]',
    })
  })

  it('runs the check again when retry is requested', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(new Response(null, { status: 200 }))
    const { result } = renderHook(() =>
      useConnectionCheck({ enabled: true, onUnauthorized: vi.fn() }),
    )

    await waitFor(() => expect(result.current.ready).toBe(true))
    act(() => result.current.retry())
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
  })
})
