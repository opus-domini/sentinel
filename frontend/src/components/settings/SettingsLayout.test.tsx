// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

import SettingsLayout from './SettingsLayout'

const navigate = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    params,
    ...props
  }: {
    children: ReactNode
    to: string
    params?: { section?: string }
  }) => (
    <a href={params?.section ? to.replace('$section', params.section) : to} {...props}>
      {children}
    </a>
  ),
  Outlet: () => <div>Section outlet</div>,
  useNavigate: () => navigate,
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: '/settings/storage' } }),
}))

vi.mock('@/components/layout/AppShell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({ hostname: 'sentinel-test' }),
}))

afterEach(cleanup)

describe('SettingsLayout', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    navigate.mockReset()
  })

  it('renders one scroll owner with delivered deep links and active state', () => {
    render(<SettingsLayout />)

    expect(screen.getAllByTestId('settings-scroll-owner')).toHaveLength(1)
    const mobileNavigation = screen.getByRole('combobox', { name: 'Settings section' })
    expect(mobileNavigation.textContent).toContain('Storage')
    expect(mobileNavigation.textContent).toContain('Usage and protected history')

    const navigation = screen.getByRole('navigation', { name: 'Settings sections' })
    const links = within(navigation).getAllByRole('link')
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      '/settings/experience',
      '/settings/operations',
      '/settings/integrations',
      '/settings/accounts',
      '/settings/access',
      '/settings/storage',
      '/settings/diagnostics',
    ])
    expect(screen.getByRole('link', { name: /Storage/ }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByText('Section outlet')).toBeTruthy()
  })

  it('navigates from the compact section selector', async () => {
    render(<SettingsLayout />)

    fireEvent.click(screen.getByRole('combobox', { name: 'Settings section' }))
    fireEvent.click(await screen.findByRole('option', { name: /Operations/ }))

    expect(navigate).toHaveBeenCalledWith({
      to: '/settings/$section',
      params: { section: 'operations' },
    })
  })
})
