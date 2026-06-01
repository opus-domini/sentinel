// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import DismissAlertDialog from './DismissAlertDialog'

describe('DismissAlertDialog', () => {
  afterEach(() => cleanup())

  it('identifies the alert, supports cancel, and confirms only on action', () => {
    const onOpenChange = vi.fn()
    const onConfirm = vi.fn()

    render(
      <DismissAlertDialog
        open
        alertTitle="Disk recovered"
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />,
    )

    expect(screen.getByRole('alertdialog')).not.toBeNull()
    expect(screen.getByText('Disk recovered')).not.toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(onConfirm).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })
})
