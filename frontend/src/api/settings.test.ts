// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

import { getSettings, patchSettings } from './settings'
import { createSettingsSnapshot } from '@/test/settings'

describe('settings API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('retains the response ETag and sends it back through If-Match', async () => {
    const snapshot = createSettingsSnapshot()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(snapshot, 200))
      .mockResolvedValueOnce(jsonResponse(snapshot, 200))
    vi.stubGlobal('fetch', fetchMock)

    const loaded = await getSettings()
    expect(loaded.etag).toBe(snapshot.etag)
    await patchSettings(loaded.etag, { experience: { locale: 'pt-BR' } })

    const patchInit = fetchMock.mock.calls[1]?.[1] as RequestInit
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/ops/settings')
    expect(patchInit.method).toBe('PATCH')
    expect(new Headers(patchInit.headers).get('If-Match')).toBe(snapshot.etag)
    expect(patchInit.body).toBe(JSON.stringify({ experience: { locale: 'pt-BR' } }))
  })

  it('maps the typed error envelope without exposing an untyped response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: {
              code: 'CONFIG_CONFLICT',
              message: 'settings changed since they were loaded',
            },
          }),
          {
            status: 412,
            headers: { 'Content-Type': 'application/json' },
          },
        ),
      ),
    )

    await expect(getSettings()).rejects.toMatchObject({
      status: 412,
      code: 'CONFIG_CONFLICT',
      message: 'settings changed since they were loaded',
    })
  })
})

function jsonResponse(
  snapshot: ReturnType<typeof createSettingsSnapshot>,
  status: number,
): Response {
  return new Response(JSON.stringify({ data: snapshot.settings }), {
    status,
    headers: {
      'Content-Type': 'application/json',
      ETag: snapshot.etag,
    },
  })
}
