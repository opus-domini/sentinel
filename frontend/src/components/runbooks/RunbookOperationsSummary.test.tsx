// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import type { OpsRunbook, OpsRunbookRun, OpsSchedule } from '@/types'
import { TooltipProvider } from '@/components/ui/tooltip'
import { RunbookOperationsSummary } from './RunbookOperationsSummary'

afterEach(() => {
  cleanup()
})

function runbook(overrides: Partial<OpsRunbook> = {}): OpsRunbook {
  return {
    id: 'rb-1',
    name: 'Restart API',
    description: 'Restart the api service',
    enabled: true,
    parameters: [],
    steps: [],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function job(overrides: Partial<OpsRunbookRun> = {}): OpsRunbookRun {
  return {
    id: 'job-1',
    runbookId: 'rb-1',
    runbookName: 'Restart API',
    status: 'succeeded',
    totalSteps: 2,
    completedSteps: 2,
    currentStep: '',
    error: '',
    stepResults: [],
    createdAt: '2026-01-01T10:00:00Z',
    startedAt: '2026-01-01T10:00:00Z',
    finishedAt: '2026-01-01T10:00:30Z',
    ...overrides,
  }
}

function schedule(overrides: Partial<OpsSchedule> = {}): OpsSchedule {
  return {
    id: 'schedule-1',
    runbookId: 'rb-1',
    name: 'Daily restart',
    scheduleType: 'cron',
    cronExpr: '0 3 * * *',
    timezone: 'UTC',
    runAt: '',
    enabled: true,
    lastRunAt: '',
    lastRunStatus: '',
    nextRunAt: '2026-01-02T03:00:00Z',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderSummary(props: ComponentProps<typeof RunbookOperationsSummary>) {
  return render(
    <TooltipProvider>
      <RunbookOperationsSummary {...props} />
    </TooltipProvider>,
  )
}

describe('RunbookOperationsSummary', () => {
  it('opens the runbook behind actionable summary cards', () => {
    const onSelectRunbook = vi.fn()

    renderSummary({
      runbooks: [
        runbook({ id: 'rb-active', name: 'Deploy API' }),
        runbook({ id: 'rb-approval', name: 'Promote release' }),
        runbook({ id: 'rb-failed', name: 'Restart API' }),
        runbook({ id: 'rb-scheduled', name: 'Rotate logs' }),
      ],
      jobs: [
        job({
          id: 'active-job',
          runbookId: 'rb-active',
          runbookName: 'Deploy API',
          status: 'running',
          currentStep: 'Deploy release',
          finishedAt: '',
          createdAt: '2026-01-01T11:00:00Z',
        }),
        job({
          id: 'approval-job',
          runbookId: 'rb-approval',
          runbookName: 'Promote release',
          status: 'waiting_approval',
          completedSteps: 2,
          currentStep: 'Approve production deploy',
          finishedAt: '',
          createdAt: '2026-01-01T10:30:00Z',
        }),
        job({
          id: 'failed-job',
          runbookId: 'rb-failed',
          runbookName: 'Restart API',
          status: 'failed',
          createdAt: '2026-01-01T10:00:00Z',
        }),
      ],
      schedules: [
        schedule({
          runbookId: 'rb-scheduled',
          name: 'Rotate logs overnight',
          nextRunAt: '2026-01-02T02:00:00Z',
        }),
      ],
      selectedRunbookId: null,
      onSelectRunbook,
    })

    fireEvent.click(screen.getByRole('button', { name: /Active runs: 1/i }))
    fireEvent.click(
      screen.getByRole('button', { name: /Pending approvals: 1/i }),
    )
    fireEvent.click(
      screen.getByRole('button', { name: /Failed last runs: 1/i }),
    )
    fireEvent.click(screen.getByRole('button', { name: /Scheduled: 1/i }))

    expect(onSelectRunbook).toHaveBeenNthCalledWith(1, 'rb-active')
    expect(onSelectRunbook).toHaveBeenNthCalledWith(2, 'rb-approval')
    expect(onSelectRunbook).toHaveBeenNthCalledWith(3, 'rb-failed')
    expect(onSelectRunbook).toHaveBeenNthCalledWith(4, 'rb-scheduled')
  })

  it('keeps empty summary cards disabled', () => {
    const onSelectRunbook = vi.fn()

    renderSummary({
      runbooks: [runbook()],
      jobs: [],
      schedules: [],
      selectedRunbookId: null,
      onSelectRunbook,
    })

    const failedCard = screen.getByRole('button', {
      name: /Failed last runs: 0/i,
    })

    expect(failedCard.getAttribute('aria-disabled')).toBe('true')
    fireEvent.click(failedCard)
    expect(onSelectRunbook).not.toHaveBeenCalled()
  })

  it('keeps healthy and failed cards scoped to their own categories', () => {
    renderSummary({
      runbooks: [
        runbook({ id: 'rb-ok', name: 'Successful backup' }),
        runbook({ id: 'rb-failed', name: 'Restart database' }),
      ],
      jobs: [
        job({
          id: 'ok-job',
          runbookId: 'rb-ok',
          runbookName: 'Successful backup',
          status: 'succeeded',
          createdAt: '2026-01-01T10:00:00Z',
        }),
        job({
          id: 'failed-job',
          runbookId: 'rb-failed',
          runbookName: 'Restart database',
          status: 'failed',
          createdAt: '2026-01-01T11:00:00Z',
        }),
      ],
      schedules: [],
      selectedRunbookId: null,
      onSelectRunbook: vi.fn(),
    })

    expect(
      screen.getByRole('button', {
        name: /Failed last runs: 1. Restart database/i,
      }),
    ).toBeTruthy()
    expect(
      screen.getByRole('button', {
        name: /Healthy runbooks: 1. Successful backup/i,
      }),
    ).toBeTruthy()
  })
})
