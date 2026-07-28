// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SidebarShell from './SidebarShell'
import { LayoutContext } from '@/contexts/LayoutContext'

const state = vi.hoisted(() => ({
  tokenRequired: true,
  authenticated: true,
  setToken: vi.fn(),
}))

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: true,
    touchCapable: true,
    touchOptimized: true,
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

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...rest }: { children: ReactNode; to: string }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: '/tmux' } }),
}))

const layoutValue = {
  sidebarOpen: true,
  setSidebarOpen: () => {},
  sidebarCollapsed: false,
  setSidebarCollapsed: () => {},
  sidebarDensity: 'full' as const,
  sidebarWidth: 340,
  sidebarMinWidth: 240,
  sidebarMaxWidth: 440,
  shellStyle: {},
  layoutGridClass: '',
  startSidebarResize: () => {},
  resizeSidebarBy: () => {},
  resizeSidebarTo: () => {},
}

afterEach(() => {
  cleanup()
  state.tokenRequired = true
  state.authenticated = true
  state.setToken.mockReset()
})

describe('SidebarShell', () => {
  it('wraps sidebar content in a bounded scroll region below the mobile nav', () => {
    const { container } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SidebarShell isOpen collapsed={false}>
          <div>Sidebar content</div>
        </SidebarShell>
      </LayoutContext.Provider>,
    )

    expect(screen.getByLabelText('Close menu')).toBeTruthy()
    expect(
      Array.from(container.querySelectorAll('aside button')).map((button) =>
        button.getAttribute('aria-label'),
      ),
    ).toEqual(['Close menu', 'About Terminal', 'API token'])
    for (const button of container.querySelectorAll('aside button')) {
      expect(button.className).toContain('size-8')
    }

    const aside = container.querySelector('aside')
    expect(aside?.className).toContain('flex-col')
    expect(aside?.className).toContain('overflow-hidden')

    const contentWrapper = screen.getByText('Sidebar content').parentElement
    expect(contentWrapper?.className).toContain('min-h-0')
    expect(contentWrapper?.className).toContain('flex-1')
    expect(contentWrapper?.className).toContain('flex')
    expect(contentWrapper?.className).toContain('flex-col')
    expect(contentWrapper?.className).toContain('overflow-hidden')
    expect(contentWrapper?.className).toContain('outline-none')
  })

  it('opens authentication from mobile and hides it when the server has no token', () => {
    const { rerender } = render(
      <LayoutContext.Provider value={layoutValue}>
        <SidebarShell isOpen collapsed={false}>
          <div>Sidebar content</div>
        </SidebarShell>
      </LayoutContext.Provider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'API token' }))
    expect(screen.getByRole('heading', { name: 'Authentication token' })).toBeTruthy()

    state.tokenRequired = false
    rerender(
      <LayoutContext.Provider value={layoutValue}>
        <SidebarShell isOpen collapsed={false}>
          <div>Sidebar content</div>
        </SidebarShell>
      </LayoutContext.Provider>,
    )

    expect(screen.queryByRole('button', { name: 'API token' })).toBeNull()
    expect(screen.getByRole('button', { name: 'About Terminal' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull()
  })
})
