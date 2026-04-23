// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionLaunchersDialog from './SessionLaunchersDialog'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    processUser: 'hugo',
    isRoot: false,
    canSwitchUser: true,
    allowedUsers: ['hugo', 'postgres'],
  }),
}))

describe('SessionLaunchersDialog', () => {
  afterEach(() => {
    cleanup()
  })

  it('saves a new session launcher', async () => {
    const onSave = vi.fn().mockResolvedValue(true)

    render(
      <SessionLaunchersDialog
        open
        onOpenChange={vi.fn()}
        defaultCwd="/srv"
        presets={[]}
        onSave={onSave}
        onDelete={vi.fn().mockResolvedValue(true)}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'api app' },
    })
    fireEvent.change(screen.getByLabelText('Working Directory'), {
      target: { value: '/srv/api' },
    })
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Icon' }), {
      button: 0,
      ctrlKey: false,
    })
    fireEvent.click(await screen.findByText('Server'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith({
        previousName: '',
        name: 'api-app',
        cwd: '/srv/api',
        icon: 'server',
        user: '',
      })
    })
  })

  it('deletes an existing session launcher', async () => {
    const onDelete = vi.fn().mockResolvedValue(true)

    render(
      <SessionLaunchersDialog
        open
        onOpenChange={vi.fn()}
        defaultCwd="/srv"
        presets={[
          {
            name: 'api',
            cwd: '/srv/api',
            icon: 'server',
            user: 'postgres',
            createdAt: '2026-04-23T00:00:00Z',
            updatedAt: '2026-04-23T00:00:00Z',
            lastLaunchedAt: '',
            launchCount: 0,
          },
        ]}
        onSave={vi.fn().mockResolvedValue(true)}
        onDelete={onDelete}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('api').closest('button')!)
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    const confirmButton = await screen.findByRole('button', { name: 'Delete' })
    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith('api')
    })
  })
})
