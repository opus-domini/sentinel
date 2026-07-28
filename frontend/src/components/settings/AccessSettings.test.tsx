// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AccessSettings from './AccessSettings'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  refetch: vi.fn(),
  pushToast: vi.fn(),
  settings: null as ReturnType<typeof createSettingsSnapshot>['settings'] | null,
  isLoading: false,
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
    isSaving: mocks.isSaving,
    error: mocks.error,
  }),
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: mocks.pushToast }),
}))

describe('AccessSettings', () => {
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
    mocks.isSaving = false
    mocks.error = null
  })

  afterEach(cleanup)

  it('requires a reinforced review and submits the derived reconnect origin', async () => {
    mocks.save.mockResolvedValue(
      createSettingsSnapshot({
        access: { port: 5050 },
      }),
    )
    render(<AccessSettings />)

    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5050' } })
    expect(mocks.blockerOptions?.enableBeforeUnload).toBe(true)
    expect(screen.getByText('http://127.0.0.1:5050')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Review and save' }))
    const dialog = screen.getByRole('alertdialog')
    expect(dialog.textContent).toContain('Save this access boundary?')
    expect(dialog.textContent).toContain('server.port')
    expect(dialog.textContent).toContain('http://127.0.0.1:5050')
    expect(mocks.save).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Save guarded candidate' }))
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        access: {
          reconnectOrigin: 'http://127.0.0.1:5050',
          port: 5050,
        },
      }),
    )
  })

  it('removes a submitted token from the DOM before the request completes', async () => {
    let resolveSave: ((snapshot: ReturnType<typeof createSettingsSnapshot>) => void) | undefined
    mocks.save.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve
      }),
    )
    render(<AccessSettings />)

    const actionGroup = screen.getByRole('group', { name: 'Shared token action' })
    fireEvent.click(within(actionGroup).getByRole('button', { name: 'Replace' }))
    fireEvent.change(screen.getByLabelText('New shared token'), {
      target: { value: 'browser-private-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Review and save' }))
    const dialog = screen.getByRole('alertdialog')
    expect(dialog.textContent).toContain('stop authenticating after restart')
    expect(dialog.textContent).not.toContain('browser-private-token')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save guarded candidate' }))

    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        access: {
          reconnectOrigin: 'http://127.0.0.1:4040',
          token: { action: 'replace', value: 'browser-private-token' },
        },
      }),
    )
    expect(screen.queryByLabelText('New shared token')).toBeNull()
    expect(document.body.textContent).not.toContain('browser-private-token')

    resolveSave?.(
      createSettingsSnapshot({
        access: {
          tokenConfigured: true,
          runtimeTokenConfigured: false,
        },
        tokenRestartPending: true,
      }),
    )
    expect(await screen.findByText(/Access settings saved/)).toBeTruthy()
  })

  it('blocks remote token clear and missing origins before opening confirmation', () => {
    render(<AccessSettings />)
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '0.0.0.0' } })
    const actionGroup = screen.getByRole('group', { name: 'Shared token action' })
    fireEvent.click(within(actionGroup).getByRole('button', { name: 'Clear' }))
    fireEvent.click(screen.getByRole('button', { name: 'Review and save' }))

    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(screen.getByText(/remote listener cannot clear/i)).toBeTruthy()
    expect(screen.getByText(/requires at least one allowed origin/i)).toBeTruthy()
    expect(mocks.save).not.toHaveBeenCalled()
  })

  it('surfaces bind preflight errors and preserves non-secret drafts', async () => {
    mocks.save.mockRejectedValue(
      new ApiError('one or more settings are invalid', 422, 'CONFIG_INVALID', {
        issues: ['server listener preflight could not bind candidate address "127.0.0.1:5050"'],
      }),
    )
    render(<AccessSettings />)

    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5050' } })
    fireEvent.click(screen.getByRole('button', { name: 'Review and save' }))
    fireEvent.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Save guarded candidate',
      }),
    )

    expect(await screen.findAllByText(/could not bind candidate address/)).not.toHaveLength(0)
    expect((screen.getByLabelText('Port') as HTMLInputElement).value).toBe('5050')
  })

  it('guards navigation and keeps environment-owned access fields read-only', () => {
    mocks.settings = createSettingsSnapshot({
      access: {
        allowedOriginsSource: 'environment',
        allowedOriginsEditable: false,
      },
    }).settings
    render(<AccessSettings />)

    expect((screen.getByLabelText('New origin') as HTMLInputElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5050' } })
    expect(
      mocks.blockerOptions?.shouldBlockFn({
        current: { pathname: '/settings/access' },
        next: { pathname: '/settings/diagnostics' },
      }),
    ).toBe(true)
  })
})
