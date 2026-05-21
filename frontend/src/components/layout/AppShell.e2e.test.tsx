// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AppShell from './AppShell'
import { LayoutContext } from '@/contexts/LayoutContext'
import { useShellLayout } from '@/hooks/useShellLayout'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/components/settings/SettingsDialog', () => ({
  default: ({ open }: { open: boolean }) =>
    open ? (
      <div role="dialog" aria-label="Settings">
        Settings
      </div>
    ) : null,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...rest }: { children: ReactNode; to: string }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
  useRouterState: ({
    select,
  }: {
    select: (state: { location: { pathname: string } }) => string
  }) => select({ location: { pathname: '/tmux' } }),
}))

vi.mock('@/hooks/useEdgeSwipe', () => ({
  useEdgeSwipe: vi.fn(),
}))

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

function ShellHarness() {
  const layout = useShellLayout({
    storageKey: 'sentinel_e2e_sidebar',
    defaultSidebarWidth: 340,
    minSidebarWidth: 240,
    maxSidebarWidth: 440,
  })

  return (
    <LayoutContext.Provider value={layout}>
      <AppShell sidebar={<aside>Sessions</aside>}>
        <main>Terminal</main>
      </AppShell>
    </LayoutContext.Provider>
  )
}

function sidebarSeparator() {
  return screen.getByRole('separator', { name: 'Resize sidebar' })
}

describe('AppShell integrated shell flow', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  afterEach(() => {
    cleanup()
  })

  it('resizes the sidebar through keyboard controls and persists the width', async () => {
    render(<ShellHarness />)

    expect(sidebarSeparator().getAttribute('aria-valuenow')).toBe('340')

    fireEvent.keyDown(sidebarSeparator(), { key: 'End' })

    await waitFor(() => {
      expect(sidebarSeparator().getAttribute('aria-valuenow')).toBe('440')
    })
    expect(window.localStorage.getItem('sentinel_e2e_sidebar_width')).toBe(
      '440',
    )

    fireEvent.keyDown(sidebarSeparator(), { key: 'ArrowLeft', shiftKey: true })

    await waitFor(() => {
      expect(sidebarSeparator().getAttribute('aria-valuenow')).toBe('400')
    })
    expect(window.localStorage.getItem('sentinel_e2e_sidebar_width')).toBe(
      '400',
    )
  })

  it('opens settings from the side rail and removes the resizer when collapsed', () => {
    render(<ShellHarness />)

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))

    expect(
      screen.queryByRole('separator', { name: 'Resize sidebar' }),
    ).toBeNull()
    expect(
      screen
        .getByRole('button', { name: 'Expand sidebar' })
        .getAttribute('aria-expanded'),
    ).toBe('false')
  })
})
