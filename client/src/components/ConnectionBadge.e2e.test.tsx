// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import ConnectionBadge from '@/components/ConnectionBadge'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

describe('ConnectionBadge', () => {
  it('renders compact status semaphore with accessible label', () => {
    render(<ConnectionBadge state="connected" />)
    const badge = screen.getByLabelText('Connected')
    expect(badge).toBeTruthy()
    expect(badge.textContent).toBe('')

    const dot = badge.querySelector('span')
    expect(dot).not.toBeNull()
  })
})
