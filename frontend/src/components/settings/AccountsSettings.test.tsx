// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AccountsSettings from './AccountsSettings'
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

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({
    pushToast: mocks.pushToast,
  }),
}))

describe('AccountsSettings', () => {
  beforeEach(() => {
    mocks.settings = createSettingsSnapshot().settings
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
    mocks.save.mockResolvedValue(
      createSettingsSnapshot({
        accounts: {
          allowedUsers: ['hugo'],
        },
      }),
    )
  })

  afterEach(cleanup)

  it('filters the closed inventory and saves only the resulting allowlist', async () => {
    render(<AccountsSettings />)
    expect(screen.getAllByText('All detected accounts').length).toBeGreaterThan(0)
    expect(screen.getByRole('checkbox', { name: 'Disallow deploy' })).toBeTruthy()
    expect(screen.getByRole('checkbox', { name: 'Disallow hugo' })).toBeTruthy()
    expect(
      (screen.getByRole('checkbox', { name: 'Allow root' }) as HTMLButtonElement).disabled,
    ).toBe(true)

    fireEvent.change(screen.getByLabelText('Allowed OS accounts'), {
      target: { value: 'deploy' },
    })
    const inventory = screen.getByLabelText('Detected OS accounts')
    expect(within(inventory).getAllByRole('checkbox')).toHaveLength(1)
    fireEvent.click(within(inventory).getByRole('checkbox', { name: 'Disallow deploy' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        accounts: {
          allowedUsers: ['hugo'],
        },
      }),
    )
  })

  it('requires explicit root confirmation and removes root when disabled', async () => {
    mocks.settings = createSettingsSnapshot({
      accounts: {
        allowedUsers: ['deploy'],
      },
    }).settings
    mocks.save.mockResolvedValue(
      createSettingsSnapshot({
        accounts: {
          allowedUsers: ['deploy', 'root'],
          allowRootTarget: true,
        },
      }),
    )
    render(<AccountsSettings />)

    fireEvent.click(screen.getByRole('switch', { name: 'Allow root targeting' }))
    expect(screen.getByRole('alertdialog').textContent).toContain('Allow Sentinel to target root?')
    fireEvent.click(screen.getByRole('button', { name: 'I understand, allow root' }))
    expect(
      screen.getByRole('switch', { name: 'Allow root targeting' }).getAttribute('aria-checked'),
    ).toBe('true')
    expect(screen.getByRole('checkbox', { name: 'Disallow root' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        accounts: {
          allowedUsers: ['deploy', 'root'],
          allowRootTarget: true,
        },
      }),
    )
  })

  it('keeps environment-owned controls read-only and guards dirty navigation', () => {
    mocks.settings = createSettingsSnapshot({
      accounts: {
        allowedUsersSource: 'environment',
        allowedUsersEditable: false,
        methodSource: 'environment',
        methodEditable: false,
      },
    }).settings
    render(<AccountsSettings />)

    expect(
      (screen.getByRole('checkbox', { name: 'Disallow deploy' }) as HTMLButtonElement).disabled,
    ).toBe(true)
    expect((screen.getByLabelText('User switch method') as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getAllByText(/owned by the environment/)).toHaveLength(2)

    fireEvent.click(screen.getByRole('switch', { name: 'Allow root targeting' }))
    fireEvent.click(screen.getByRole('button', { name: 'I understand, allow root' }))
    expect(mocks.blockerOptions?.enableBeforeUnload).toBe(true)
    expect(
      mocks.blockerOptions?.shouldBlockFn({
        current: { pathname: '/settings/accounts' },
        next: { pathname: '/settings/operations' },
      }),
    ).toBe(true)
  })

  it('renders deterministic loading and error states', () => {
    mocks.settings = null
    mocks.isLoading = true
    render(<AccountsSettings />)
    expect(screen.getByLabelText('Loading account settings')).toBeTruthy()

    cleanup()
    mocks.isLoading = false
    mocks.isError = true
    mocks.error = new Error('settings offline')
    render(<AccountsSettings />)
    expect(screen.getByRole('alert').textContent).toContain('settings offline')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})
