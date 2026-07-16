// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import TokenDialog from './TokenDialog'

afterEach(() => {
  cleanup()
})

describe('TokenDialog', () => {
  it('awaits clearing the authenticated cookie before closing', async () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn().mockResolvedValue({ ok: true, status: 204 })

    render(
      <TokenDialog
        open
        onOpenChange={onOpenChange}
        authenticated
        tokenRequired
        onTokenChange={onTokenChange}
      />,
    )

    expect(screen.getByText('Authenticated')).toBeTruthy()
    expect(screen.queryByPlaceholderText('token (required)')).toBeNull()
    expect(screen.getAllByRole('button', { name: 'Close' }).length).toBe(2)
    expect(screen.getByRole('button', { name: 'Clear cookie' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Clear cookie' }))

    expect(onTokenChange).toHaveBeenCalledWith('')
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it('closes only after token save succeeds', async () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn().mockResolvedValue({ ok: true, status: 204 })

    render(
      <TokenDialog
        open
        onOpenChange={onOpenChange}
        authenticated={false}
        tokenRequired
        onTokenChange={onTokenChange}
      />,
    )

    const input = screen.getByPlaceholderText('token (required)')
    const saveButton = screen.getByRole('button', { name: 'Save' })

    expect(screen.queryByText('Authenticated')).toBeNull()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeTruthy()
    expect(saveButton).toHaveProperty('disabled', true)

    fireEvent.change(input, { target: { value: 'secret-token' } })
    expect(saveButton).toHaveProperty('disabled', false)

    fireEvent.click(saveButton)

    expect(onTokenChange).toHaveBeenCalledWith('secret-token')
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it('stays open and shows an inline error when token is rejected', async () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn().mockResolvedValue({ ok: false, status: 401 })

    render(
      <TokenDialog
        open
        onOpenChange={onOpenChange}
        authenticated={false}
        tokenRequired
        onTokenChange={onTokenChange}
      />,
    )

    const input = screen.getByPlaceholderText('token (required)')
    fireEvent.change(input, { target: { value: 'bad-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect((await screen.findByRole('alert')).textContent).toContain('Invalid token.')
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toBe(screen.getByText('Invalid token.').id)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })

  it('shows an origin-specific error when the token endpoint is blocked by origin policy', async () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      code: 'ORIGIN_DENIED',
      message: 'request origin is not allowed',
    })

    render(
      <TokenDialog
        open
        onOpenChange={onOpenChange}
        authenticated={false}
        tokenRequired
        onTokenChange={onTokenChange}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('token (required)'), {
      target: { value: 'correct-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'request origin is not allowed',
    )
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
