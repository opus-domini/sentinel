// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import IntegrationsSettings from './IntegrationsSettings'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  refetch: vi.fn(),
  pushToast: vi.fn(),
  settings: null as ReturnType<typeof createSettingsSnapshot>['settings'] | null,
  isLoading: false,
  isError: false,
  isSaving: false,
  error: null as Error | null,
  blocker: {
    status: 'idle' as 'idle' | 'blocked',
    proceed: vi.fn(),
    reset: vi.fn(),
  },
  blockerOptions: null as null | {
    shouldBlockFn: (args: { current: { pathname: string }; next: { pathname: string } }) => boolean
    enableBeforeUnload: boolean
  },
}))

vi.mock('@tanstack/react-router', () => ({
  useBlocker: (options: typeof mocks.blockerOptions) => {
    mocks.blockerOptions = options
    return mocks.blocker
  },
}))

vi.mock('@/hooks/useSettings', () => ({
  useSettings: () => ({
    settings: mocks.settings,
    save: mocks.save,
    refetch: mocks.refetch,
    isLoading: mocks.isLoading,
    isError: mocks.isError,
    isSaving: mocks.isSaving,
    error: mocks.error,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({ hostname: 'test-host' }),
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: mocks.pushToast }),
}))

describe('IntegrationsSettings', () => {
  beforeEach(() => {
    mocks.settings = createSettingsSnapshot({
      tokenConfigured: true,
      runtimeTokenConfigured: true,
      webhookConfigured: false,
    }).settings
    mocks.save.mockReset()
    mocks.refetch.mockReset()
    mocks.pushToast.mockReset()
    mocks.blocker.proceed.mockReset()
    mocks.blocker.reset.mockReset()
    mocks.blocker.status = 'idle'
    mocks.blockerOptions = null
    mocks.isLoading = false
    mocks.isError = false
    mocks.isSaving = false
    mocks.error = null
  })

  afterEach(cleanup)

  it('submits the write-only webhook and removes it from the DOM before completion', async () => {
    let resolveSave: ((snapshot: ReturnType<typeof createSettingsSnapshot>) => void) | undefined
    mocks.save.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve
      }),
    )
    render(<IntegrationsSettings />)

    const webhookSection = screen.getByText('Delivery webhook').closest('section')
    if (webhookSection == null) throw new Error('missing webhook section')

    fireEvent.click(screen.getByLabelText('Enable MCP'))
    fireEvent.change(screen.getByLabelText('Delivery schedule'), {
      target: { value: '0 * * * *' },
    })
    fireEvent.click(within(webhookSection).getByRole('button', { name: 'Replace' }))
    fireEvent.change(screen.getByLabelText('New webhook url'), {
      target: { value: 'https://hooks.example.test/test-webhook-value' },
    })

    const unsaved = screen.getByRole('complementary', { name: 'Unsaved settings' })
    expect(unsaved.textContent).toContain(
      'health_report.webhook_url · Not configured → Replacement staged',
    )
    expect(unsaved.textContent).not.toContain('test-webhook-value')

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        integrations: {
          mcp: {
            enabled: true,
          },
          healthReport: {
            schedule: '0 * * * *',
            webhookUrl: {
              action: 'replace',
              value: 'https://hooks.example.test/test-webhook-value',
            },
          },
        },
      }),
    )
    expect(screen.queryByLabelText('New webhook url')).toBeNull()
    expect(document.body.textContent).not.toContain('test-webhook-value')

    resolveSave?.(
      createSettingsSnapshot({
        mcpEnabled: true,
        tokenConfigured: true,
        runtimeTokenConfigured: true,
        webhookConfigured: true,
        webhookRestartPending: true,
        healthSchedule: '0 * * * *',
        nextActivation: '2026-07-28T02:00:00-03:00',
      }),
    )
    expect(await screen.findByText(/Integration settings saved/)).toBeTruthy()
  })

  it('maps backend cron errors and discards replacement values on failure', async () => {
    mocks.save.mockRejectedValue(
      new ApiError('one or more settings are invalid', 422, 'CONFIG_INVALID', {
        issues: ['health_report.schedule invalid cron expression'],
      }),
    )
    render(<IntegrationsSettings />)

    fireEvent.change(screen.getByLabelText('Delivery schedule'), {
      target: { value: 'not a cron' },
    })
    const webhookSection = screen.getByText('Delivery webhook').closest('section')
    if (webhookSection == null) throw new Error('missing webhook section')
    fireEvent.click(within(webhookSection).getByRole('button', { name: 'Replace' }))
    fireEvent.change(screen.getByLabelText('New webhook url'), {
      target: { value: 'https://hooks.example.test/rejected-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(await screen.findByText('health_report.schedule invalid cron expression')).toBeTruthy()
    expect(screen.queryByLabelText('New webhook url')).toBeNull()
    expect(document.body.textContent).not.toContain('rejected-value')
    expect((screen.getByLabelText('Delivery schedule') as HTMLInputElement).value).toBe(
      'not a cron',
    )
  })

  it('guards navigation and discards drafts without a PATCH', () => {
    render(<IntegrationsSettings />)
    fireEvent.change(screen.getByLabelText('Delivery schedule'), {
      target: { value: '@daily' },
    })

    expect(mocks.blockerOptions?.enableBeforeUnload).toBe(true)
    expect(
      mocks.blockerOptions?.shouldBlockFn({
        current: { pathname: '/settings/integrations' },
        next: { pathname: '/settings/operations' },
      }),
    ).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect((screen.getByLabelText('Delivery schedule') as HTMLInputElement).value).toBe('')
    expect(mocks.save).not.toHaveBeenCalled()
  })

  it('renders the environment-owned webhook as read-only', () => {
    mocks.settings = createSettingsSnapshot({
      webhookSource: 'environment',
      webhookEditable: false,
    }).settings
    render(<IntegrationsSettings />)

    expect(screen.getAllByText(/owned by the environment/)).toHaveLength(1)
    const webhookSection = screen.getByText('Delivery webhook').closest('section')
    if (webhookSection == null) throw new Error('missing webhook section')
    expect(
      (within(webhookSection).getByRole('button', { name: 'Replace' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true)
  })
})
