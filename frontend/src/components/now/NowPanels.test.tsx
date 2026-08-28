// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import type { NowSnapshot } from '@/types'
import { NowAttention, primaryMetricSignal } from './NowAttention'
import { NowInProgress } from './NowInProgress'
import { NowStatus } from './NowStatus'

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
  confidence: {
    state: 'degraded',
    sources: {
      services: { status: 'current', observedAt: '2026-07-27T12:00:00Z' },
      metrics: {
        status: 'unavailable',
        observedAt: '2026-07-27T12:00:00Z',
        message: 'collector unavailable',
      },
      runbooks: { status: 'stale', observedAt: '2026-07-27T12:00:00Z' },
      tmux: { status: 'not_configured', observedAt: '2026-07-27T12:00:00Z' },
    },
  },
  posture: {
    state: 'unknown',
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
    overflow: { approvals: 1, services: 1, metrics: 4 },
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
}

describe('Now panels', () => {
  it('keeps valid data visible while naming every partial source state', () => {
    render(<NowStatus snapshot={snapshot} />)

    expect(screen.getByText('Unknown')).toBeTruthy()
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
          overflow: { approvals: 0, services: 0, metrics: 0 },
        }}
        degraded={false}
        onRunProcedure={vi.fn()}
      />,
    )

    expect(screen.getByText('No action needed')).toBeTruthy()

    rerender(
      <NowInProgress
        runs={[
          {
            id: 'job-running',
            runbookId: 'runbook-1',
            runbookName: 'Recover Sentinel',
            status: 'running',
            totalSteps: 2,
            completedSteps: 1,
            source: 'now',
            createdAt: '2026-07-27T11:58:00Z',
          },
        ]}
        sessions={snapshot.inProgress.sessions}
      />,
    )
    expect(screen.getByRole('link', { name: /Recover Sentinel/ }).getAttribute('href')).toBe(
      '/runbooks?job=job-running',
    )
    expect(screen.getByRole('link', { name: /sentinel-dev/ }).getAttribute('href')).toBe(
      '/tmux?session=sentinel-dev',
    )
  })

  it('opens a pending approval by its execution receipt alone', () => {
    render(
      <NowAttention
        attention={{
          total: 1,
          visible: [
            {
              type: 'runbook_approval',
              run: {
                runbookId: 'runbook-1',
                runbookName: 'Recover Sentinel',
                runId: 'job-approval',
                status: 'waiting_approval',
                createdAt: '2026-07-27T11:58:00Z',
              },
            },
          ],
          overflow: { approvals: 0, services: 0, metrics: 0 },
        }}
        degraded={false}
        onRunProcedure={vi.fn()}
      />,
    )

    expect(screen.getByRole('link', { name: /Approval waiting/ }).getAttribute('href')).toBe(
      '/runbooks?job=job-approval',
    )
  })

  it('hands host pressure to the highest-severity Metrics signal without guessing an owner', () => {
    const signals = [
      {
        name: 'cpu' as const,
        severity: 'warning' as const,
        value: 82,
        since: '2026-07-27T11:58:00Z',
      },
      {
        name: 'ioPressure' as const,
        severity: 'critical' as const,
        value: 64,
        since: '2026-07-27T11:59:00Z',
      },
    ]
    expect(primaryMetricSignal(signals)?.name).toBe('ioPressure')

    render(
      <NowAttention
        attention={{
          total: 1,
          visible: [
            {
              type: 'metrics_pressure',
              severity: 'critical',
              signals,
              observedAt: '2026-07-27T12:00:00Z',
            },
          ],
          overflow: { approvals: 0, services: 0, metrics: 0 },
        }}
        degraded={false}
        onRunProcedure={vi.fn()}
      />,
    )

    const href = screen.getByRole('link', { name: /Host pressure/ }).getAttribute('href')
    expect(href).toBe('/metrics?signal=ioPressure&focusAt=2026-07-27T12%3A00%3A00Z')
    expect(href).not.toContain('service')
    expect(href).not.toContain('process')
  })

  it('names the hardware sensor subject in the decision queue', () => {
    render(
      <NowAttention
        attention={{
          total: 1,
          visible: [
            {
              type: 'metrics_pressure',
              severity: 'critical',
              signals: [
                {
                  name: 'temperature',
                  subject: 'Package id 0 (coretemp)',
                  severity: 'critical',
                  value: 96,
                  since: '2026-08-28T12:00:00Z',
                },
              ],
              observedAt: '2026-08-28T12:00:00Z',
            },
          ],
          overflow: { approvals: 0, services: 0, metrics: 0 },
        }}
        degraded={false}
        onRunProcedure={vi.fn()}
      />,
    )

    expect(screen.getByText('Temperature: Package id 0 (coretemp)')).toBeTruthy()
    expect(screen.getByRole('link', { name: /Host pressure/ }).getAttribute('href')).toBe(
      '/metrics?signal=temperature&focusAt=2026-08-28T12%3A00%3A00Z',
    )
  })
})
