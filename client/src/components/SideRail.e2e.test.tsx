// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import SideRail from '@/components/SideRail'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
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
  }) => select({ location: { pathname: '/services' } }),
}))

vi.mock('@/components/settings/SettingsDialog', () => ({
  default: () => null,
}))

describe('SideRail', () => {
  it('keeps desktop side rail icon-only with accessible labels', () => {
    const { container } = render(
      <SideRail sidebarCollapsed={false} onToggleSidebarCollapsed={() => {}} />,
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

    const desktopSettingsButton = aside?.querySelector(
      'button[aria-label="Settings"]',
    )
    expect(desktopSettingsButton).not.toBeNull()
  })
})
