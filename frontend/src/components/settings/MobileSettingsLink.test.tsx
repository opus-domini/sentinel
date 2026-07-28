// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

import MobileSettingsLink from './MobileSettingsLink'

const viewport = vi.hoisted(() => ({ compactLayout: false }))

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: viewport.compactLayout,
    touchCapable: viewport.compactLayout,
    touchOptimized: viewport.compactLayout,
  }),
}))

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))

afterEach(() => {
  cleanup()
  viewport.compactLayout = false
})

describe('MobileSettingsLink', () => {
  it('adds a 44px global settings target only in compact headers', () => {
    const { rerender } = render(<MobileSettingsLink />)
    expect(screen.queryByRole('link', { name: 'Settings' })).toBeNull()

    viewport.compactLayout = true
    rerender(<MobileSettingsLink />)
    const link = screen.getByRole('link', { name: 'Settings' })
    expect(link.getAttribute('href')).toBe('/settings')
    expect(link.className).toContain('size-11')
  })
})
