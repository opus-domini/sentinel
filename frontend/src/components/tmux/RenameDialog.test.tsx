// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import RenameDialog from './RenameDialog'

describe('RenameDialog', () => {
  afterEach(() => cleanup())

  it('stays open and shows an inline error when submit rejects', async () => {
    const onOpenChange = vi.fn()
    const onSubmit = vi.fn().mockRejectedValue(new Error('Rename failed on server'))

    render(
      <RenameDialog
        open
        onOpenChange={onOpenChange}
        title="Rename session"
        description="Choose a new name."
        value="dev"
        onValueChange={vi.fn()}
        onSubmit={onSubmit}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    expect((await screen.findByRole('alert')).textContent).toContain('Rename failed on server')
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })

  it('closes after submit resolves', async () => {
    const onOpenChange = vi.fn()
    const onSubmit = vi.fn().mockResolvedValue(undefined)

    render(
      <RenameDialog
        open
        onOpenChange={onOpenChange}
        title="Rename session"
        description="Choose a new name."
        value="dev"
        onValueChange={vi.fn()}
        onSubmit={onSubmit}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })
})
