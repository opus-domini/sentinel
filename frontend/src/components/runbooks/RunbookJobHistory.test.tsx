// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { OpsRunbookRun } from '@/types'
import { RunbookJobHistory } from './RunbookJobHistory'

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatDateTime: (value: string) => value,
  }),
}))

afterEach(() => {
  cleanup()
})

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

describe('RunbookJobHistory', () => {
  it('filters the operational history by job state', () => {
    render(
      <RunbookJobHistory
        jobs={[
          job({ id: 'ok', status: 'succeeded' }),
          job({
            id: 'bad',
            status: 'failed',
            error: 'systemctl failed',
            finishedAt: '2026-01-01T10:00:05Z',
          }),
          job({
            id: 'active',
            status: 'running',
            completedSteps: 1,
            currentStep: 'Restart service',
            finishedAt: '',
          }),
        ]}
        onDeleteJob={vi.fn()}
        onApproveJob={vi.fn()}
        onRejectJob={vi.fn()}
      />,
    )

    expect(screen.getByText('succeeded')).toBeTruthy()
    expect(screen.getByText('failed')).toBeTruthy()
    expect(screen.getByText('running')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Failed 1' }))

    expect(screen.queryByText('succeeded')).toBeNull()
    expect(screen.getByText('failed')).toBeTruthy()
    expect(screen.queryByText('running')).toBeNull()
  })

  it('surfaces waiting approvals and sends approve or reject actions', async () => {
    const onApproveJob = vi.fn().mockResolvedValue(undefined)
    const onRejectJob = vi.fn().mockResolvedValue(undefined)

    render(
      <RunbookJobHistory
        jobs={[
          job({
            id: 'approval',
            status: 'waiting_approval',
            completedSteps: 2,
            totalSteps: 3,
            currentStep: 'Approve restart',
            finishedAt: '',
            stepResults: [
              {
                stepIndex: 0,
                title: 'Check status',
                type: 'run',
                output: 'service is degraded',
                error: '',
                durationMs: 120,
              },
              {
                stepIndex: 1,
                title: 'Approve restart',
                type: 'approval',
                output: 'Confirm restart after reviewing status.',
                error: '',
                durationMs: 0,
              },
            ],
          }),
        ]}
        onDeleteJob={vi.fn()}
        onApproveJob={onApproveJob}
        onRejectJob={onRejectJob}
      />,
    )

    expect(screen.getByText('Waiting approval')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Delete job' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Approvals 1' }))
    expect(screen.getByText('Waiting approval')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Approve run' }))
    await waitFor(() => expect(onApproveJob).toHaveBeenCalledWith('approval'))

    fireEvent.click(screen.getByRole('button', { name: 'Reject approval' }))
    fireEvent.click(screen.getByRole('button', { name: 'Reject' }))

    await waitFor(() => expect(onRejectJob).toHaveBeenCalledWith('approval'))
  })
})
