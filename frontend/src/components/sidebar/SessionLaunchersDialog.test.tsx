// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SessionLaunchersDialog from './SessionLaunchersDialog'

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: false,
    touchCapable: false,
    touchOptimized: false,
  }),
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

  it('saves a new session launcher', async () => {
    const onSave = vi.fn().mockResolvedValue('launcher-api')

    render(
      <SessionLaunchersDialog
        open
        onOpenChange={vi.fn()}
        defaultCwd="/srv"
        launchers={[]}
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
        id: '',
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
        launchers={[
          {
            id: 'launcher-api',
            name: 'api',
            cwd: '/srv/api',
            icon: 'server',
            user: 'postgres',
            createdAt: '2026-04-23T00:00:00Z',
            updatedAt: '2026-04-23T00:00:00Z',
            lastUsedAt: '',
            useCount: 0,
          },
        ]}
        onSave={vi.fn().mockResolvedValue('launcher-api')}
        onDelete={onDelete}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('api').closest('button')!)
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    const confirmButton = await screen.findByRole('button', { name: 'Delete' })
    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith('launcher-api')
    })
  })

  it('suggests directories for the working directory field', async () => {
    globalThis.fetch = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/api/fs/dirs')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { dirs: ['/srv/app', '/srv/api'] } }),
        })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ data: { dirs: [] } }) })
    }) as typeof globalThis.fetch

    render(
      <SessionLaunchersDialog
        open
        onOpenChange={vi.fn()}
        defaultCwd="/srv"
        launchers={[]}
        onSave={vi.fn().mockResolvedValue('launcher-api')}
        onDelete={vi.fn().mockResolvedValue(true)}
        onReorder={vi.fn()}
      />,
    )

    const cwd = screen.getByRole('combobox', { name: 'Working Directory' })
    fireEvent.focus(cwd)
    fireEvent.change(cwd, { target: { value: '/srv' } })

    const option = await screen.findByRole('option', { name: '/srv/app' })
    fireEvent.keyDown(cwd, { key: 'ArrowDown' })

    expect(cwd.getAttribute('aria-activedescendant')).toBe(option.id)
    expect(option.getAttribute('aria-selected')).toBe('true')
  })
})
