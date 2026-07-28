// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

import SettingsLayout from './SettingsLayout'

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
  it('renders one scroll owner with delivered deep links and active state', () => {
    render(<SettingsLayout />)

    expect(screen.getAllByTestId('settings-scroll-owner')).toHaveLength(1)
    const navigation = screen.getByRole('navigation', { name: 'Settings sections' })
    const links = within(navigation).getAllByRole('link')
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      '/settings/experience',
      '/settings/operations',
      '/settings/integrations',
      '/settings/accounts',
      '/settings/storage',
      '/settings/diagnostics',
    ])
    expect(screen.getByRole('link', { name: /Storage/ }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByText('Section outlet')).toBeTruthy()
  })
})
