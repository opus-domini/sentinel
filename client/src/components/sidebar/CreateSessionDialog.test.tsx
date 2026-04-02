// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import CreateSessionDialog from './CreateSessionDialog'

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    processUser: 'hugo',
    isRoot: false,
    canSwitchUser: true,
    allowedUsers: ['hugo', 'postgres'],
  }),
}))

describe('CreateSessionDialog', () => {
  const originalFetch = globalThis.fetch

  beforeEach(() => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: { dirs: [] } }),
    }) as typeof globalThis.fetch
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    cleanup()
  })

  it('shows advanced options toggle when canSwitchUser is true', () => {
    render(
      <CreateSessionDialog
        open
        onOpenChange={vi.fn()}
        defaultCwd=""
        onCreate={vi.fn()}
      />,
    )

    expect(screen.getByText('Advanced options')).toBeTruthy()
  })

  it('creates sessions with name and working directory', async () => {
    const onOpenChange = vi.fn()
    const onCreate = vi.fn()

    render(
      <CreateSessionDialog
        open
        onOpenChange={onOpenChange}
        defaultCwd=""
        onCreate={onCreate}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('session name'), {
      target: { value: 'dev' },
    })
    fireEvent.change(screen.getByPlaceholderText('working directory'), {
      target: { value: '/tmp' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    expect(onCreate).toHaveBeenCalledWith('dev', '/tmp', undefined)
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })
})
