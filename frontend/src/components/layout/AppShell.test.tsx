// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import AppShell from './AppShell'
import { LayoutContext } from '@/contexts/LayoutContext'
import type { ReactNode } from 'react'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...rest }: { children: ReactNode; to: string }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: '/services' } }),
}))

vi.mock('@/components/SideRail', () => ({
  default: () => <nav aria-label="side rail" />,
}))

vi.mock('@/components/settings/SettingsDialog', () => ({
  default: () => null,
}))

vi.mock('@/hooks/useEdgeSwipe', () => ({
  useEdgeSwipe: vi.fn(),
}))

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

function renderShell(overrides = {}) {
  const layoutValue = {
    sidebarOpen: false,
    setSidebarOpen: vi.fn(),
    sidebarCollapsed: false,
    setSidebarCollapsed: vi.fn(),
    sidebarDensity: 'full' as const,
    sidebarWidth: 340,
    sidebarMinWidth: 240,
    sidebarMaxWidth: 440,
    settingsOpen: false,
    setSettingsOpen: vi.fn(),
    shellStyle: {},
    layoutGridClass:
      'grid h-full grid-cols-[1fr] grid-rows-[minmax(0,1fr)] md:[grid-template-columns:48px_var(--sidebar-width)_6px_1fr]',
    startSidebarResize: vi.fn(),
    resizeSidebarBy: vi.fn(),
    resizeSidebarTo: vi.fn(),
    ...overrides,
  }

  render(
    <LayoutContext.Provider value={layoutValue}>
      <AppShell sidebar={<aside>Sessions</aside>}>
        <main>Terminal</main>
      </AppShell>
    </LayoutContext.Provider>,
  )

  return layoutValue
}

describe('AppShell', () => {
  afterEach(() => {
    cleanup()
  })

  it('exposes the sidebar resizer as a keyboard-operable separator', () => {
    const layout = renderShell()

    const separator = screen.getByRole('separator', {
      name: 'Resize sidebar',
    })

    expect(separator.getAttribute('aria-orientation')).toBe('vertical')
    expect(separator.getAttribute('aria-valuemin')).toBe('240')
    expect(separator.getAttribute('aria-valuemax')).toBe('440')
    expect(separator.getAttribute('aria-valuenow')).toBe('340')

    fireEvent.keyDown(separator, { key: 'ArrowRight' })
    fireEvent.keyDown(separator, { key: 'ArrowLeft', shiftKey: true })
    fireEvent.keyDown(separator, { key: 'Home' })
    fireEvent.keyDown(separator, { key: 'End' })

    expect(layout.resizeSidebarBy).toHaveBeenNthCalledWith(1, 16)
    expect(layout.resizeSidebarBy).toHaveBeenNthCalledWith(2, -40)
    expect(layout.resizeSidebarTo).toHaveBeenNthCalledWith(1, 240)
    expect(layout.resizeSidebarTo).toHaveBeenNthCalledWith(2, 440)
  })

  it('does not resize the sidebar for modified arrow shortcuts', () => {
    const layout = renderShell()
    const separator = screen.getByRole('separator', {
      name: 'Resize sidebar',
    })

    fireEvent.keyDown(separator, { key: 'ArrowRight', ctrlKey: true })
    fireEvent.keyDown(separator, { key: 'ArrowLeft', metaKey: true })
    fireEvent.keyDown(separator, { key: 'ArrowRight', altKey: true })

    expect(layout.resizeSidebarBy).not.toHaveBeenCalled()
    expect(layout.resizeSidebarTo).not.toHaveBeenCalled()
  })

  it('renders persistent mobile primary navigation in shared order with active page state', () => {
    const layout = renderShell()

    const nav = screen.getByRole('navigation', {
      name: 'Mobile primary navigation',
    })
    const links = Array.from(nav.querySelectorAll('a'))

    expect(nav.className).toContain('grid-cols-4')
    expect(nav.className).toContain('inset-x-0')
    expect(nav.className).toContain('bottom-0')
    expect(nav.className).toContain('mobile-primary-nav')
    expect(nav.className).toContain('border-t')
    expect(nav.className).not.toContain('rounded-2xl')
    expect(nav.className).not.toContain('shadow-2xl')
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      '/tmux',
      '/services',
      '/runbooks',
      '/metrics',
    ])
    expect(links.map((link) => link.getAttribute('aria-label'))).toEqual([
      'Tmux',
      'Services',
      'Runbooks',
      'Metrics',
    ])
    const activeLink = screen.getByRole('link', { name: 'Services' })
    const activeIcon = activeLink.querySelector('svg')

    expect(activeLink.getAttribute('aria-current')).toBe('page')
    expect(activeLink.className).toContain('text-primary/60')
    expect(activeLink.className).not.toContain('bg-primary')
    expect(activeLink.className).not.toContain('ring-primary')
    expect(activeLink.className).toContain('py-0.5')
    expect(activeIcon?.getAttribute('class')).not.toContain('text-primary-text-bright')
    expect(activeIcon?.getAttribute('class')).toContain('size-3.5')

    fireEvent.click(activeLink)
    expect(layout.setSidebarOpen).toHaveBeenCalledWith(true)
  })
})
