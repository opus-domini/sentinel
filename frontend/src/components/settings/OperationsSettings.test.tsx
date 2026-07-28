// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import OperationsSettings from './OperationsSettings'
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

describe('OperationsSettings', () => {
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
        operations: {
          captureLines: 120,
        },
      }),
    )
  })

  afterEach(cleanup)

  it('validates ranges before save and sends only changed typed fields', async () => {
    render(<OperationsSettings />)
    const input = screen.getByLabelText('Capture lines') as HTMLInputElement

    fireEvent.change(input, { target: { value: '2001' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'Capture lines must be between 1 and 2,000.',
    )
    expect(mocks.save).not.toHaveBeenCalled()

    fireEvent.change(input, { target: { value: '120' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith({
        operations: {
          watchtower: {
            captureLines: 120,
          },
        },
      }),
    )
    expect(await screen.findByText(/Operational settings saved/)).toBeTruthy()
  })

  it('discards the draft without PATCH and configures internal and beforeunload guards', async () => {
    render(<OperationsSettings />)
    const input = screen.getByLabelText('Concurrent runbooks') as HTMLInputElement

    fireEvent.change(input, { target: { value: '9' } })
    expect(await screen.findByRole('complementary', { name: 'Unsaved settings' })).toBeTruthy()
    expect(mocks.blockerOptions?.enableBeforeUnload).toBe(true)
    expect(
      mocks.blockerOptions?.shouldBlockFn({
        current: { pathname: '/settings/operations' },
        next: { pathname: '/settings/diagnostics' },
      }),
    ).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect(input.value).toBe('5')
    expect(mocks.save).not.toHaveBeenCalled()
  })

  it('keeps a conflict draft and offers stay or discard for blocked navigation', async () => {
    mocks.blocker.status = 'blocked'
    mocks.save.mockRejectedValue({
      code: 'CONFIG_CONFLICT',
      message: 'settings changed',
    })
    render(<OperationsSettings />)
    const input = screen.getByLabelText('Journal rows') as HTMLInputElement
    fireEvent.change(input, { target: { value: '6000' } })

    expect(screen.getByRole('alertdialog')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Stay' }))
    expect(mocks.blocker.reset).toHaveBeenCalledOnce()

    fireEvent.click(screen.getByRole('button', { name: 'Discard and leave' }))
    expect(mocks.blocker.proceed).toHaveBeenCalledOnce()
    expect(input.value).toBe('5000')
  })

  it('keeps touched fields while adopting untouched values after a conflict refetch', async () => {
    const view = render(<OperationsSettings />)
    fireEvent.change(screen.getByLabelText('Capture lines'), { target: { value: '120' } })

    mocks.settings = createSettingsSnapshot({
      revision: 'b'.repeat(64),
      operations: {
        maxConcurrent: 8,
      },
    }).settings
    view.rerender(<OperationsSettings />)

    await waitFor(() =>
      expect((screen.getByLabelText('Capture lines') as HTMLInputElement).value).toBe('120'),
    )
    expect((screen.getByLabelText('Concurrent runbooks') as HTMLInputElement).value).toBe('8')
    const changes = screen.getByRole('complementary', { name: 'Unsaved settings' })
    expect(changes.textContent).toContain('watchtower.capture_lines · 80 → 120')
    expect(changes.textContent).not.toContain('runbooks.max_concurrent')
  })

  it('renders reproducible loading and error states', () => {
    mocks.settings = null
    mocks.isLoading = true
    render(<OperationsSettings />)
    expect(screen.getByLabelText('Loading operational settings')).toBeTruthy()

    cleanup()
    mocks.isLoading = false
    mocks.isError = true
    mocks.error = new Error('settings offline')
    render(<OperationsSettings />)
    expect(screen.getByRole('alert').textContent).toContain('settings offline')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})
