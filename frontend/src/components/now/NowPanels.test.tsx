// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import type { NowSnapshot } from '@/types'
import { NowAttention } from './NowAttention'
import { NowInProgress } from './NowInProgress'
import { NowReliability } from './NowReliability'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
    ...rest
  }: {
    children: ReactNode
    to: string
    search?: Record<string, string>
  }) => {
    const query = search ? `?${new URLSearchParams(search)}` : ''
    return (
      <a href={`${to}${query}`} {...rest}>
        {children}
      </a>
    )
  },
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatRelativeTime: (value: string) => value,
  }),
}))

afterEach(cleanup)

const snapshot: NowSnapshot = {
  generatedAt: '2026-07-27T12:00:00Z',
  reliability: {
    state: 'degraded',
    services: { tracked: 3, running: 2, failed: 1, inactive: 0, unknown: 0 },
    metrics: {
      state: 'pressure',
      severity: 'warning',
      warningCount: 1,
      criticalCount: 0,
      observedAt: '2026-07-27T12:00:00Z',
      signals: [
        {
          name: 'cpu',
          severity: 'warning',
          value: 82,
          since: '2026-07-27T11:59:50Z',
        },
      ],
    },
  },
  attention: {
    total: 7,
    visible: [
      {
        type: 'service_failed',
        service: {
          name: 'sentinel',
          displayName: 'Sentinel',
          manager: 'systemd',
          scope: 'user',
          unit: 'sentinel.service',
          trackingMode: 'custom',
        },
      },
    ],
    overflow: { approvals: 1, services: 1, runbooks: 0, metrics: 4 },
  },
  inProgress: {
    runs: [],
    sessions: [
      {
        name: 'sentinel-dev',
        user: 'operator',
        unreadPanes: 2,
        unreadWindows: 1,
        pinned: true,
        activityAt: '2026-07-27T11:59:00Z',
      },
    ],
  },
  sources: {
    services: { status: 'current', checkedAt: '2026-07-27T12:00:00Z' },
    metrics: {
      status: 'unavailable',
      checkedAt: '2026-07-27T12:00:00Z',
      message: 'collector unavailable',
    },
    runbooks: { status: 'stale', checkedAt: '2026-07-27T12:00:00Z' },
    tmux: { status: 'not_configured', checkedAt: '2026-07-27T12:00:00Z' },
  },
}

describe('Now panels', () => {
  it('keeps valid data visible while naming every partial source state', () => {
    render(<NowReliability snapshot={snapshot} />)

    expect(screen.getByText('Degraded')).toBeTruthy()
    expect(screen.getByLabelText('Services: Current')).toBeTruthy()
    expect(screen.getByLabelText('Metrics: Unavailable')).toBeTruthy()
    expect(screen.getByLabelText('Runbooks: Stale')).toBeTruthy()
    expect(screen.getByLabelText('Tmux: Not configured')).toBeTruthy()
  })

  it('links visible and overflow attention to their owner modules', () => {
    render(<NowAttention attention={snapshot.attention} degraded onRunProcedure={vi.fn()} />)

    expect(screen.getByRole('link', { name: /Sentinel is failed/ }).getAttribute('href')).toBe(
      '/services?service=sentinel&panel=status',
    )
    expect(screen.getByText('6 more in owner modules:')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Runbooks 1' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Services 1' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Metrics 4' })).toBeTruthy()
  })

  it('shows calm empty copy and deep-links existing live context', () => {
    const { rerender } = render(
      <NowAttention
        attention={{
          total: 0,
          visible: [],
          overflow: { approvals: 0, services: 0, runbooks: 0, metrics: 0 },
        }}
        degraded={false}
        onRunProcedure={vi.fn()}
      />,
    )

    expect(screen.getByText('No action needed')).toBeTruthy()

    rerender(<NowInProgress runs={[]} sessions={snapshot.inProgress.sessions} />)
    expect(screen.getByRole('link', { name: /sentinel-dev/ }).getAttribute('href')).toBe(
      '/tmux?session=sentinel-dev',
    )
  })
})
