// @vitest-environment jsdom
import { renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useTmuxApi } from './useTmuxApi'

describe('useTmuxApi', () => {
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

  it('returns unwrapped data on success', async () => {
    mockFetch(200, { data: { sessions: ['a', 'b'] } })

    const { result } = renderHook(() => useTmuxApi(''))
    const data = await result.current<{ sessions: Array<string> }>(
      '/api/tmux/sessions',
    )

    expect(data).toEqual({ sessions: ['a', 'b'] })
  })

  it('sends bearer token when provided', async () => {
    mockFetch(200, { data: {} })

    const { result } = renderHook(() => useTmuxApi('my-secret'))
    await result.current('/api/tmux/sessions')

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBe('Bearer my-secret')
  })

  it('does not send authorization header when token is empty', async () => {
    mockFetch(200, { data: {} })

    const { result } = renderHook(() => useTmuxApi(''))
    await result.current('/api/tmux/sessions')

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBeUndefined()
  })

  it('trims whitespace from token', async () => {
    mockFetch(200, { data: {} })

    const { result } = renderHook(() => useTmuxApi('  tok  '))
    await result.current('/api/test')

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBe('Bearer tok')
  })

  it('does not send auth for whitespace-only token', async () => {
    mockFetch(200, { data: {} })

    const { result } = renderHook(() => useTmuxApi('   '))
    await result.current('/api/test')

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers.Authorization).toBeUndefined()
  })

  it('throws with error message from response body', async () => {
    mockFetch(400, { error: { message: 'invalid session name' } })

    const { result } = renderHook(() => useTmuxApi(''))
    await expect(result.current('/api/tmux/sessions')).rejects.toThrow(
      'invalid session name',
    )
  })

  it('throws with HTTP status when no error message in body', async () => {
    mockFetch(500, {})

    const { result } = renderHook(() => useTmuxApi(''))
    await expect(result.current('/api/test')).rejects.toThrow('HTTP 500')
  })

  it('returns empty object when response has no data key', async () => {
    mockFetch(200, { something: 'else' })

    const { result } = renderHook(() => useTmuxApi(''))
    const data = await result.current('/api/test')

    expect(data).toEqual({})
  })

  it('merges custom headers from init', async () => {
    mockFetch(200, { data: {} })

    const { result } = renderHook(() => useTmuxApi('tok'))
    await result.current('/api/test', {
      headers: { 'X-Custom': 'value' },
    })

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[1].headers['X-Custom']).toBe('value')
    expect(call[1].headers['Content-Type']).toBe('application/json')
  })

  it('passes method and body from init', async () => {
    mockFetch(201, { data: { name: 'new' } })

    const { result } = renderHook(() => useTmuxApi(''))
    await result.current('/api/tmux/sessions', {
      method: 'POST',
      body: JSON.stringify({ name: 'new' }),
    })

    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[0]).toBe('/api/tmux/sessions')
    expect(call[1].method).toBe('POST')
    expect(call[1].body).toBe('{"name":"new"}')
  })

  it('handles non-JSON error response gracefully', async () => {
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    })

    const { result } = renderHook(() => useTmuxApi(''))
    await expect(result.current('/api/test')).rejects.toThrow('HTTP 502')
  })
})
