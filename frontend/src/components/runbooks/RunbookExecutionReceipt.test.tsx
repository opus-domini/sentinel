// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { OpsRunbookRun, OpsServiceStatusResponse } from '@/types'
import { RunbookExecutionReceipt } from './RunbookExecutionReceipt'

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
}))

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
  }) => (
    <a href={`${to}?${new URLSearchParams(search)}`} {...rest}>
      {children}
    </a>
  ),
}))

vi.mock('@/hooks/useTmuxApi', () => ({
  useTmuxApi: () => mocks.api,
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatDateTime: (value: string) => `absolute:${value}`,
    formatRelativeTime: (value: string) => `relative:${value}`,
  }),
}))

function execution(overrides: Partial<OpsRunbookRun> = {}): OpsRunbookRun {
  return {
    id: 'job-1',
    runbookId: 'runbook-1',
    runbookName: 'Current name must not win',
    status: 'succeeded',
    totalSteps: 1,
    completedSteps: 1,
    currentStep: '',
    error: '',
    source: 'now',
    targetKind: 'service',
    targetName: 'sentinel',
    parametersUsed: { MODE: 'safe' },
    definition: {
      schemaVersion: 1,
      runbookId: 'runbook-1',
      name: 'Frozen recovery',
      description: 'Recorded before execution',
      parameters: [
        {
          name: 'MODE',
          label: 'Mode',
          type: 'select',
          default: 'safe',
          required: true,
          options: ['safe', 'force'],
        },
      ],
      webhookURL: '',
      targetKind: 'service',
      targetName: 'sentinel',
      steps: [{ type: 'run', title: 'Frozen restart', command: 'systemctl restart sentinel' }],
    },
    stepResults: [
      {
        stepIndex: 0,
        title: 'Frozen restart',
        type: 'run',
        output: 'restart requested',
        error: '',
        durationMs: 42,
      },
    ],
    createdAt: '2026-07-27T10:00:00Z',
    startedAt: '2026-07-27T10:00:01Z',
    finishedAt: '2026-07-27T10:00:02Z',
    ...overrides,
  }
}

function renderReceipt(job: OpsRunbookRun) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RunbookExecutionReceipt job={job} standalone />
    </QueryClientProvider>,
  )
}

describe('RunbookExecutionReceipt', () => {
  beforeEach(() => {
    mocks.api.mockReset()
  })

  afterEach(cleanup)

  it('separates immutable execution result from current target state', async () => {
    const response: OpsServiceStatusResponse = {
      status: {
        service: {
          name: 'sentinel',
          displayName: 'Sentinel',
          trackingMode: 'custom',
          manager: 'systemd',
          scope: 'user',
          unit: 'sentinel.service',
          exists: true,
          enabledState: 'enabled',
          activeState: 'failed',
          updatedAt: '2026-07-27T10:05:00Z',
        },
        summary: 'failed',
        condition: {
          activeState: 'failed',
          subState: 'failed',
          result: 'exit-code',
          exitStatus: 1,
        },
        observedAt: '2026-07-27T10:05:00Z',
      },
      context: { runbook: null, latestRun: null },
    }
    mocks.api.mockResolvedValue(response)

    renderReceipt(execution())

    expect(screen.getByText('Immutable execution receipt')).toBeTruthy()
    expect(screen.getByText('Frozen recovery')).toBeTruthy()
    expect(screen.getByText('Schema 1')).toBeTruthy()
    expect(screen.getByText('Now')).toBeTruthy()
    expect(screen.getByText('succeeded')).toBeTruthy()
    expect(screen.getByText('MODE')).toBeTruthy()
    expect(screen.getByText('safe')).toBeTruthy()
    expect(screen.getByText('Frozen restart')).toBeTruthy()
    expect(screen.getByText('restart requested')).toBeTruthy()
    expect(screen.getByRole('link', { name: /sentinel/ }).getAttribute('href')).toBe(
      '/services?service=sentinel&panel=status',
    )

    await waitFor(() => expect(screen.getByText('failed')).toBeTruthy())
    expect(screen.getByText('Observed relative:2026-07-27T10:05:00Z')).toBeTruthy()
    expect(mocks.api).toHaveBeenCalledWith('/api/ops/services/sentinel/status')
  })

  it('names the incomplete evidence available for a legacy execution', () => {
    renderReceipt(
      execution({
        definition: undefined,
        targetKind: undefined,
        targetName: undefined,
        parametersUsed: undefined,
      }),
    )

    expect(screen.getByText('Legacy receipt')).toBeTruthy()
    expect(screen.getByText('Definition snapshot unavailable')).toBeTruthy()
    expect(mocks.api).not.toHaveBeenCalled()
  })
})
