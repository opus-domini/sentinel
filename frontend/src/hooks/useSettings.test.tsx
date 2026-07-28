// @vitest-environment jsdom
import type { PropsWithChildren } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useSettings } from './useSettings'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  patchSettings: vi.fn(),
}))

vi.mock('@/api/settings', () => ({
  getSettings: mocks.getSettings,
  patchSettings: mocks.patchSettings,
}))

describe('useSettings', () => {
  beforeEach(() => {
    mocks.getSettings.mockReset()
    mocks.patchSettings.mockReset()
  })

  it('refetches a conflict while leaving draft ownership with the caller', async () => {
    const initial = createSettingsSnapshot({ locale: 'en-US' })
    const refreshed = createSettingsSnapshot({
      revision: 'b'.repeat(64),
      locale: 'pt-BR',
    })
    mocks.getSettings.mockResolvedValueOnce(initial).mockResolvedValue(refreshed)
    mocks.patchSettings.mockRejectedValue(
      new ApiError('settings changed since they were loaded', 412, 'CONFIG_CONFLICT'),
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useSettings(), { wrapper })
    await waitFor(() =>
      expect(result.current.settings?.experience.locale.effectiveValue).toBe('en-US'),
    )

    let saveError: unknown
    await act(async () => {
      try {
        await result.current.save({ experience: { locale: 'en-GB' } })
      } catch (error) {
        saveError = error
      }
    })

    expect(saveError).toMatchObject({ code: 'CONFIG_CONFLICT' })
    await waitFor(() => {
      expect(mocks.getSettings).toHaveBeenCalledTimes(2)
      expect(result.current.settings?.experience.locale.effectiveValue).toBe('pt-BR')
    })
  })
})
