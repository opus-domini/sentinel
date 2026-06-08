// @vitest-environment jsdom
import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { authCookieUpdateErrorMessage, updateAuthCookie } from './authToken'

describe('auth token helpers', () => {
  const originalFetch = globalThis.fetch
  let queryClient: QueryClient

  beforeEach(() => {
    globalThis.fetch = vi.fn()
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
          gcTime: 0,
        },
      },
    })
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    queryClient.clear()
  })

  it('sends token validation with same-origin credentials', async () => {
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response(JSON.stringify({ data: { authenticated: true } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await updateAuthCookie(queryClient, ' secret-token ')

    expect(result).toEqual({ ok: true, status: 200, code: '', message: '' })
    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/auth/token',
      expect.objectContaining({
        method: 'PUT',
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ token: 'secret-token' }),
      }),
    )
  })

  it('keeps auth error code and message from the token endpoint', async () => {
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            code: 'ORIGIN_DENIED',
            message: 'request origin is not allowed',
          },
        }),
        {
          status: 403,
          headers: { 'Content-Type': 'application/json' },
        },
      ),
    )

    const result = await updateAuthCookie(queryClient, 'secret-token')

    expect(result).toEqual({
      ok: false,
      status: 403,
      code: 'ORIGIN_DENIED',
      message: 'request origin is not allowed',
    })
    expect(authCookieUpdateErrorMessage(result)).toBe(
      'Request origin is not allowed. Update server.allowed_origins for this URL.',
    )
  })

  it('uses the invalid token message only for validation 401s', () => {
    expect(authCookieUpdateErrorMessage({ ok: false, status: 401, code: '', message: '' })).toBe(
      'Invalid token.',
    )
    expect(
      authCookieUpdateErrorMessage(
        { ok: false, status: 401, code: '', message: '' },
        { action: 'clear' },
      ),
    ).toBe('Unable to clear token right now. (HTTP 401).')
  })
})
