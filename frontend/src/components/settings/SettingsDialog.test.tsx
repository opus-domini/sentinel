// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsDialog from './SettingsDialog'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  pushToast: vi.fn(),
  api: vi.fn(),
  settings: null as unknown,
}))

vi.mock('@/hooks/useSettings', () => ({
  useSettings: () => ({
    settings: mocks.settings,
    snapshot: null,
    save: mocks.save,
    isLoading: false,
    error: null,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    version: 'test',
    timezone: 'UTC',
    locale: 'en-US',
    hostname: 'sentinel-test',
  }),
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({
    pushToast: mocks.pushToast,
  }),
}))

vi.mock('@/hooks/usePwaInstall', () => ({
  usePwaInstall: () => ({
    supportsPwa: true,
    installed: false,
    installAvailable: false,
    installApp: vi.fn(),
    updateAvailable: false,
    checkForUpdate: vi.fn(),
    updating: false,
  }),
}))

vi.mock('@/hooks/useTmuxApi', async (importOriginal) => {
  const original = await importOriginal()
  return {
    ...(original as object),
    useTmuxApi: () => mocks.api,
  }
})

vi.mock('@/components/settings/ThemeSelector', () => ({
  default: () => <div>Theme selector</div>,
}))

vi.mock('@/components/settings/MCPSettingsPanel', () => ({
  default: () => <div>MCP settings</div>,
}))

describe('SettingsDialog experience saves', () => {
  afterEach(cleanup)

  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    mocks.save.mockReset()
    mocks.pushToast.mockReset()
    mocks.api.mockReset()
    mocks.settings = createSettingsSnapshot().settings
    mocks.save.mockResolvedValue(createSettingsSnapshot({ timezone: 'America/Sao_Paulo' }))
  })

  it('saves a live selection and reports success', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('tab', { name: 'App' }))
    const appPanel = screen.getByRole('tabpanel', { name: 'App' })
    const timezoneSelect = within(appPanel).getAllByRole('combobox')[0]
    fireEvent.click(timezoneSelect)
    fireEvent.click(await screen.findByRole('option', { name: 'America/Sao_Paulo' }))

    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        experience: { timezone: 'America/Sao_Paulo' },
      }),
    )
    expect(mocks.pushToast).toHaveBeenCalledWith(
      expect.objectContaining({ level: 'success', title: 'Timezone saved' }),
    )
  })

  it('keeps the attempted value visible after a conflict', async () => {
    mocks.save.mockRejectedValue(
      new ApiError('settings changed since they were loaded', 412, 'CONFIG_CONFLICT'),
    )
    renderDialog()
    fireEvent.click(screen.getByRole('tab', { name: 'App' }))
    const appPanel = screen.getByRole('tabpanel', { name: 'App' })
    const timezoneSelect = within(appPanel).getAllByRole('combobox')[0]
    fireEvent.click(timezoneSelect)
    fireEvent.click(await screen.findByRole('option', { name: 'America/Sao_Paulo' }))

    expect(
      await within(appPanel).findByText('settings changed since they were loaded'),
    ).toBeTruthy()
    expect(timezoneSelect.textContent).toContain('America/Sao_Paulo')
    expect(mocks.pushToast).toHaveBeenCalledWith(
      expect.objectContaining({ level: 'error', title: 'Timezone not saved' }),
    )
  })
})

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SettingsDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  )
}
