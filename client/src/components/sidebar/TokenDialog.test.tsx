// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import TokenDialog from './TokenDialog'

afterEach(() => {
  cleanup()
})

describe('TokenDialog', () => {
  it('shows only action buttons when already authenticated', () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn()

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
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows token input and save action when unauthenticated', () => {
    const onOpenChange = vi.fn()
    const onTokenChange = vi.fn()

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
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
