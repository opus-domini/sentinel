// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionLifecycleIndicator, { formatLifecycleRemaining } from './SessionLifecycleIndicator'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ content, children }: { content: string; children: ReactNode }) => (
    <span data-tooltip={content}>{children}</span>
  ),
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatTimestamp: (value: string) => `formatted ${value}`,
  }),
}))

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('SessionLifecycleIndicator', () => {
  it('formats remaining time at minute and hour granularity', () => {
    const now = Date.parse('2026-08-01T12:00:00Z')

    expect(formatLifecycleRemaining('2026-08-01T12:01:00Z', now)).toBe('1 minute remaining')
    expect(formatLifecycleRemaining('2026-08-01T13:30:00Z', now)).toBe(
      '1 hour 30 minutes remaining',
    )
    expect(formatLifecycleRemaining('2026-08-01T12:00:00Z', now)).toBe('deadline reached')
  })

  it('describes the absolute deadline, remaining time and cleanup state without a timer', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-01T12:00:00Z'))

    const { container } = render(
      <SessionLifecycleIndicator
        lifecycle={{
          mode: 'ephemeral',
          source: 'mcp',
          cleanupState: 'grace',
          expiresAt: '2026-08-01T11:55:00Z',
          graceUntil: '2026-08-01T12:10:00Z',
        }}
      />,
    )

    expect(
      screen.getByRole('img', { name: /cleanup grace period, 10 minutes remaining/ }),
    ).toBeTruthy()
    expect(container.firstElementChild?.getAttribute('data-tooltip')).toContain(
      'Deadline: formatted 2026-08-01T12:10:00Z',
    )
    expect(vi.getTimerCount()).toBe(0)
  })
})
