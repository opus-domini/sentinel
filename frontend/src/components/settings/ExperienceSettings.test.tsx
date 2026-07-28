// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ExperienceSettings from './ExperienceSettings'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  refetch: vi.fn(),
  pushToast: vi.fn(),
  settings: null as ReturnType<typeof createSettingsSnapshot>['settings'] | null,
  isLoading: false,
  isError: false,
  error: null as Error | null,
}))

vi.mock('@/hooks/useSettings', () => ({
  useSettings: () => ({
    settings: mocks.settings,
    save: mocks.save,
    refetch: mocks.refetch,
    isLoading: mocks.isLoading,
    isError: mocks.isError,
    error: mocks.error,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    timezone: 'UTC',
    locale: 'en-US',
  }),
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({
    pushToast: mocks.pushToast,
  }),
}))

vi.mock('@/components/settings/ThemeSelector', () => ({
  default: ({ onSelect }: { onSelect: (id: string) => void }) => (
    <button type="button" onClick={() => onSelect('aurora')}>
      Aurora theme
    </button>
  ),
}))

describe('ExperienceSettings', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    window.localStorage.clear()
    mocks.save.mockReset()
    mocks.refetch.mockReset()
    mocks.pushToast.mockReset()
    mocks.settings = createSettingsSnapshot().settings
    mocks.isLoading = false
    mocks.isError = false
    mocks.error = null
    mocks.save.mockResolvedValue(createSettingsSnapshot({ timezone: 'Europe/Lisbon' }))
  })

  afterEach(cleanup)

  it('uses a closed timezone select and saves the selected option immediately', async () => {
    renderExperience()
    const timezone = screen.getByRole('combobox', { name: 'Timezone' })

    expect(screen.queryByRole('textbox', { name: 'Timezone' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Apply timezone' })).toBeNull()
    fireEvent.click(timezone)
    fireEvent.click(await screen.findByRole('option', { name: 'America/Sao_Paulo' }))

    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        experience: { timezone: 'America/Sao_Paulo' },
      }),
    )
    expect(await screen.findByText('Timezone saved.')).toBeTruthy()
  })

  it('keeps a rejected timezone draft visible', async () => {
    mocks.save.mockRejectedValue(new Error('settings changed since they were loaded'))
    renderExperience()
    const timezone = screen.getByRole('combobox', { name: 'Timezone' })

    fireEvent.click(timezone)
    fireEvent.click(await screen.findByRole('option', { name: 'America/Sao_Paulo' }))

    expect(await screen.findByText('settings changed since they were loaded')).toBeTruthy()
    expect(timezone.textContent).toContain('America/Sao_Paulo')
  })

  it('renders reproducible loading and initial error states', () => {
    mocks.settings = null
    mocks.isLoading = true
    renderExperience()
    expect(screen.getByLabelText('Loading server-managed experience settings')).toBeTruthy()

    cleanup()
    mocks.isLoading = false
    mocks.isError = true
    mocks.error = new Error('settings offline')
    renderExperience()

    expect(screen.getByRole('alert').textContent).toContain('settings offline')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})

function ExperienceHarness() {
  return <ExperienceSettings />
}

function renderExperience() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ExperienceHarness />
    </QueryClientProvider>,
  )
}
