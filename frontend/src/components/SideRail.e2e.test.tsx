// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import SideRail from '@/components/SideRail'
import { LayoutContext } from '@/contexts/LayoutContext'

const state = vi.hoisted(() => ({
  pathname: '/services',
  tokenRequired: true,
  authenticated: true,
  setToken: vi.fn(),
}))

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...rest }: { children: ReactNode; to: string }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: state.pathname } }),
}))

vi.mock('@/components/settings/SettingsDialog', () => ({
  default: () => null,
}))

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: false,
    touchCapable: false,
    touchOptimized: false,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    tokenRequired: state.tokenRequired,
  }),
}))

vi.mock('@/contexts/TokenContext', () => ({
  useTokenContext: () => ({
    authenticated: state.authenticated,
    setToken: state.setToken,
  }),
}))

const layoutValue = {
  sidebarOpen: false,
  setSidebarOpen: () => {},
  sidebarCollapsed: false,
  setSidebarCollapsed: () => {},
  sidebarDensity: 'full' as const,
  sidebarWidth: 340,
  sidebarMinWidth: 240,
  sidebarMaxWidth: 440,
  settingsOpen: false,
  setSettingsOpen: () => {},
  shellStyle: {},
  layoutGridClass: '',
  startSidebarResize: () => {},
  resizeSidebarBy: () => {},
  resizeSidebarTo: () => {},
}

afterEach(() => {
  cleanup()
  state.pathname = '/services'
  state.tokenRequired = true
  state.authenticated = true
  state.setToken.mockReset()
})

describe('SideRail', () => {
  it('keeps desktop side rail icon-only with accessible labels', () => {
    const { container } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    const aside = container.querySelector('aside')
    expect(aside).not.toBeNull()

    const desktopTmuxLink = aside?.querySelector('a[href="/tmux"]')
    expect(desktopTmuxLink).not.toBeNull()
    if (!desktopTmuxLink) {
      throw new Error('desktop tmux link not found')
    }
    expect((desktopTmuxLink.textContent || '').trim()).toBe('')
    expect(desktopTmuxLink.getAttribute('aria-label')).toBe('Tmux')

    const desktopSettingsButton = aside?.querySelector('button[aria-label="Settings"]')
    expect(desktopSettingsButton).not.toBeNull()
  })

  it('uses the shared primary nav order in the desktop rail', () => {
    const { container } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    const links = Array.from(container.querySelectorAll('aside a')).map((link) =>
      link.getAttribute('aria-label'),
    )

    expect(links).toEqual(['Now', 'Tmux', 'Runbooks', 'Services', 'Metrics'])
  })

  it('groups authentication, page help, settings, and sidebar controls at the bottom', () => {
    const { container } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    const actions = Array.from(container.querySelectorAll('aside button')).map((button) =>
      button.getAttribute('aria-label'),
    )

    expect(actions).toEqual(['API token', 'About Services', 'Settings', 'Collapse sidebar'])
  })

  it('hides authentication when the server does not require a token', () => {
    state.tokenRequired = false

    const { container } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    const actions = Array.from(container.querySelectorAll('aside button')).map((button) =>
      button.getAttribute('aria-label'),
    )

    expect(actions).toEqual(['About Services', 'Settings', 'Collapse sidebar'])
    expect(screen.queryByRole('button', { name: 'API token' })).toBeNull()
  })

  it.each([
    ['/tmux', 'About Terminal'],
    ['/runbooks', 'About Runbooks'],
    ['/services', 'About Services'],
    ['/metrics', 'About Metrics'],
  ])('shows the helper for %s', (pathname, helperLabel) => {
    state.pathname = pathname

    render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    expect(screen.getByRole('button', { name: helperLabel })).toBeTruthy()
  })

  it('opens authentication from the rail and describes the current state', () => {
    render(
      <LayoutContext.Provider value={layoutValue}>
        <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />
      </LayoutContext.Provider>,
    )

    const tokenButton = screen.getByRole('button', { name: 'API token' })
    expect(tokenButton.getAttribute('aria-description')).toBe('Authenticated (required)')

    fireEvent.click(tokenButton)

    expect(screen.getByRole('heading', { name: 'Authentication token' })).toBeTruthy()
    expect(screen.getByText('Authenticated')).toBeTruthy()
  })
})
