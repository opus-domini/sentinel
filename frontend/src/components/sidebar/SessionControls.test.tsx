// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionControls from './SessionControls'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/components/TmuxHelpDialog', () => ({
  default: ({ buttonSize }: { buttonSize?: string }) => (
    <button type="button" aria-label="About Terminal" data-size={buttonSize} />
  ),
}))

vi.mock('./CreateSessionDialog', () => ({
  default: ({ open }: { open: boolean }) => (open ? <div>Create Session Dialog</div> : null),
}))

vi.mock('./SessionLaunchersDialog', () => ({
  default: ({ open }: { open: boolean }) => (open ? <div>Session Launchers Dialog</div> : null),
}))

vi.mock('./TokenDialog', () => ({
  default: ({ open }: { open: boolean }) => (open ? <div>Token Dialog</div> : null),
}))

afterEach(() => {
  cleanup()
})

const baseProps = {
  sessionCount: 2,
  tokenRequired: false,
  authenticated: true,
  defaultCwd: '/srv',
  launchers: [],
  tmuxUnavailable: false,
  filter: '',
  onFilterChange: vi.fn(),
  onTokenChange: vi.fn(),
  onCreate: vi.fn(),
  onLaunchLauncher: vi.fn(),
  onSaveLauncher: vi.fn().mockResolvedValue('launcher-id'),
  onDeleteLauncher: vi.fn().mockResolvedValue(true),
  onReorderLaunchers: vi.fn(),
}

describe('SessionControls', () => {
  it('uses compact header controls to match the tmux window strip buttons', () => {
    render(<SessionControls {...baseProps} />)

    expect(screen.getByRole('button', { name: 'New session' }).getAttribute('data-size')).toBe(
      'icon-xs',
    )
    expect(
      screen.getByRole('button', { name: 'Open session launcher menu' }).getAttribute('data-size'),
    ).toBe('icon-xs')
    expect(screen.getByRole('button', { name: 'About Terminal' }).getAttribute('data-size')).toBe(
      'icon-xs',
    )
    expect(screen.getByRole('button', { name: 'API token' }).getAttribute('data-size')).toBe(
      'icon-xs',
    )
  })

  it('opens the create session dialog from the primary add button', () => {
    render(<SessionControls {...baseProps} />)

    fireEvent.click(screen.getByRole('button', { name: 'New session' }))

    expect(screen.getByText('Create Session Dialog')).toBeTruthy()
  })

  it('launches the recent session launcher and opens the manager from the menu', async () => {
    const onLaunchLauncher = vi.fn()

    render(
      <SessionControls
        {...baseProps}
        launchers={[
          {
            id: 'launcher-api',
            name: 'api',
            cwd: '/srv/api',
            icon: 'server',
            createdAt: '2026-04-23T00:00:00Z',
            updatedAt: '2026-04-23T00:00:00Z',
            lastUsedAt: '2026-04-23T12:00:00Z',
            useCount: 2,
          },
          {
            id: 'launcher-web',
            name: 'web',
            cwd: '/srv/web',
            icon: 'globe',
            createdAt: '2026-04-23T00:00:00Z',
            updatedAt: '2026-04-23T00:00:00Z',
            lastUsedAt: '',
            useCount: 0,
          },
        ]}
        onLaunchLauncher={onLaunchLauncher}
      />,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: 'Open session launcher menu' }), {
      button: 0,
      ctrlKey: false,
    })

    expect(await screen.findByText('Last used')).toBeTruthy()

    fireEvent.click(screen.getByText('api'))

    await waitFor(() => {
      expect(onLaunchLauncher).toHaveBeenCalledWith('launcher-api')
    })

    fireEvent.pointerDown(screen.getByRole('button', { name: 'Open session launcher menu' }), {
      button: 0,
      ctrlKey: false,
    })
    fireEvent.click(await screen.findByText('Manage session launchers...'))

    expect(screen.getByText('Session Launchers Dialog')).toBeTruthy()
  })
})
