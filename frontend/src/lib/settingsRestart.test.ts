// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'

import {
  SettingsRestartTimeoutError,
  settingsReconnectTarget,
  waitForRestartTarget,
  waitForSettingsRestart,
} from './settingsRestart'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

describe('settings restart recovery', () => {
  it('waits through the outage and returns the first applied snapshot', async () => {
    const pending = createSettingsSnapshot()
    pending.settings.restart.required = true
    const complete = createSettingsSnapshot({ revision: 'b'.repeat(64) })
    const loadSettings = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce(pending)
      .mockResolvedValueOnce(complete)

    const result = await waitForSettingsRestart({
      loadSettings,
      wait: async () => undefined,
      attempts: 3,
    })

    expect(result).toEqual({ status: 'complete', snapshot: complete })
    expect(loadSettings).toHaveBeenCalledTimes(3)
  })

  it('hands a rotated token back to the authentication gate', async () => {
    const result = await waitForSettingsRestart({
      loadSettings: async () => {
        throw new ApiError('missing or invalid token', 401, 'UNAUTHORIZED')
      },
      wait: async () => undefined,
      attempts: 1,
    })

    expect(result).toEqual({ status: 'authentication-required' })
  })

  it('times out with an actionable typed error', async () => {
    await expect(
      waitForSettingsRestart({
        loadSettings: async () => {
          throw new TypeError('Failed to fetch')
        },
        wait: async () => undefined,
        attempts: 2,
      }),
    ).rejects.toBeInstanceOf(SettingsRestartTimeoutError)
  })

  it('keeps the current path when reconnecting to a changed origin', () => {
    window.history.replaceState({}, '', '/settings/access?tab=listener#recovery')

    expect(settingsReconnectTarget('http://127.0.0.1:5050')).toBe(
      'http://127.0.0.1:5050/settings/access?tab=listener#recovery',
    )
    expect(settingsReconnectTarget(window.location.origin)).toBe('')
  })

  it('waits for a changed reconnect target before navigating', async () => {
    const probe = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce(undefined)

    await waitForRestartTarget('http://127.0.0.1:5050/settings/access', {
      probe,
      wait: async () => undefined,
      attempts: 2,
    })

    expect(probe).toHaveBeenCalledTimes(2)
    expect(probe).toHaveBeenLastCalledWith('http://127.0.0.1:5050/api/connection/check')
  })
})
