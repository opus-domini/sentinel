import { describe, expect, it } from 'vitest'

import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'
import {
  accessDraftFromSettings,
  accessErrorsFromAPI,
  accessPatchFromChanges,
  candidateReconnectOrigin,
  classifyListenerHost,
  diffAccessDraft,
  validateAccessDraft,
} from './accessDraft'

const endpoint = { protocol: 'https:', hostname: 'sentinel.example' }

describe('access draft', () => {
  it('derives specific and wildcard reconnect targets from the candidate', () => {
    const access = createSettingsSnapshot().settings.access
    const specific = { ...accessDraftFromSettings(access), host: '10.0.0.8', port: '5050' }
    expect(candidateReconnectOrigin(endpoint, specific)).toBe('https://10.0.0.8:5050')

    const wildcard = { ...specific, host: '0.0.0.0' }
    expect(candidateReconnectOrigin(endpoint, wildcard)).toBe('https://sentinel.example:5050')
    expect(classifyListenerHost('::')).toBe('wildcard')
    expect(classifyListenerHost('::1')).toBe('loopback')
  })

  it('builds one typed access patch with reconnect origin and no retained secret', () => {
    const access = createSettingsSnapshot().settings.access
    const base = accessDraftFromSettings(access)
    const draft = {
      ...base,
      port: '5050',
      tokenIntent: 'replace' as const,
      tokenValue: 'replacement-private-token',
      allowedOrigins: ['https://sentinel.example:5050'],
    }
    const changes = diffAccessDraft(base, draft, access)
    expect(accessPatchFromChanges(draft, changes, 'https://127.0.0.1:5050')).toEqual({
      access: {
        reconnectOrigin: 'https://127.0.0.1:5050',
        port: 5050,
        token: { action: 'replace', value: 'replacement-private-token' },
        allowedOrigins: ['https://sentinel.example:5050'],
      },
    })
  })

  it('blocks predictable remote, token, origin, cookie, and list lockouts', () => {
    const access = createSettingsSnapshot({
      access: { tokenConfigured: false },
    }).settings.access
    const draft = {
      ...accessDraftFromSettings(access),
      host: '0.0.0.0',
      tokenIntent: 'clear' as const,
      allowedOrigins: [],
      cookieSecure: 'never',
      trustedProxies: ['127.0.0.1', '127.0.0.1'],
    }
    expect(validateAccessDraft(draft, access, endpoint)).toMatchObject({
      token: expect.stringContaining('cannot clear'),
      allowedOrigins: expect.stringContaining('requires at least one'),
      trustedProxies: expect.stringContaining('Duplicate'),
    })
  })

  it('maps backend candidate issues to their controls', () => {
    const error = new ApiError('invalid', 422, 'CONFIG_INVALID', {
      issues: [
        'server listener preflight could not bind candidate address "127.0.0.1:5050"',
        'access.reconnectOrigin must be "http://127.0.0.1:5050"',
      ],
    })
    expect(accessErrorsFromAPI(error)).toEqual({
      host: expect.stringContaining('listener preflight'),
      port: expect.stringContaining('candidate address'),
      reconnectOrigin: expect.stringContaining('reconnectOrigin'),
    })
  })
})
