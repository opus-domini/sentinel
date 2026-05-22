// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import SidebarShell from './SidebarShell'
import { LayoutContext } from '@/contexts/LayoutContext'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
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
  settingsOpen: false,
  setSettingsOpen: () => {},
  shellStyle: {},
  layoutGridClass: '',
  startSidebarResize: () => {},
  resizeSidebarBy: () => {},
  resizeSidebarTo: () => {},
}

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
})
