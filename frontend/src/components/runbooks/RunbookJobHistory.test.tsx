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

vi.mock('@/components/runbooks/RunbookExecutionReceipt', () => ({
  RunbookExecutionReceipt: ({ job: receiptJob }: { job: OpsRunbookRun }) => (
    <div>Receipt {receiptJob.id}</div>
  ),
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
  it('shows persisted source and target without inventing historical context', () => {
    render(
      <RunbookJobHistory
        jobs={[
          job({
            id: 'contextual',
            source: 'scheduler',
            targetKind: 'service',
            targetName: 'nginx',
          }),
          job({ id: 'historical' }),
        ]}
        onDeleteJob={vi.fn()}
        onApproveJob={vi.fn()}
        onRejectJob={vi.fn()}
      />,
    )

    expect(screen.getByText('Source: scheduler · Service: nginx')).toBeTruthy()
    expect(screen.getAllByText(/Source:/)).toHaveLength(1)
  })

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
            targetKind: 'service',
            targetName: 'sentinel',
            definition: {
              schemaVersion: 1,
              runbookId: 'rb-1',
              name: 'Frozen recovery',
              description: '',
              parameters: [],
              webhookURL: '',
              targetKind: 'service',
              targetName: 'sentinel',
              steps: [
                { type: 'run', title: 'Check status', command: 'status' },
                {
                  type: 'approval',
                  title: 'Approve restart',
                  description: 'Continue?',
                },
                { type: 'run', title: 'Restart frozen target', command: 'restart' },
              ],
            },
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
    expect(screen.getByText('Target:')).toBeTruthy()
    expect(screen.getByText('3. run · Restart frozen target')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Delete job' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Approvals 1' }))
    expect(screen.getByText('Waiting approval')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Approve run' }))
    await waitFor(() => expect(onApproveJob).toHaveBeenCalledWith('approval'))

    fireEvent.click(screen.getByRole('button', { name: 'Reject approval' }))
    fireEvent.click(screen.getByRole('button', { name: 'Reject' }))

    await waitFor(() => expect(onRejectJob).toHaveBeenCalledWith('approval'))
  })

  it('expands a deep-linked job even when another filter was active', async () => {
    const failed = job({ id: 'failed', status: 'failed' })
    const approval = job({ id: 'approval', status: 'waiting_approval' })
    const props = {
      jobs: [failed, approval],
      onDeleteJob: vi.fn(),
      onApproveJob: vi.fn(),
      onRejectJob: vi.fn(),
    }
    const { rerender } = render(<RunbookJobHistory {...props} />)

    fireEvent.click(screen.getByRole('button', { name: 'Failed 1' }))
    expect(screen.queryByText('Waiting approval')).toBeNull()

    rerender(<RunbookJobHistory {...props} focusJobId="approval" />)

    await waitFor(() => {
      expect(screen.getByText('Waiting approval')).toBeTruthy()
      expect(screen.getByText('Receipt approval')).toBeTruthy()
      expect(
        screen
          .getAllByRole('button', { name: 'Toggle job details' })
          .some((button) => button.getAttribute('aria-expanded') === 'true'),
      ).toBe(true)
    })
  })

  it('keeps the canonical execution target in sync with expansion', () => {
    const onFocusJob = vi.fn()
    render(
      <RunbookJobHistory
        jobs={[job()]}
        onDeleteJob={vi.fn()}
        onApproveJob={vi.fn()}
        onRejectJob={vi.fn()}
        onFocusJob={onFocusJob}
      />,
    )

    const toggles = screen.getAllByRole('button', { name: 'Toggle job details' })
    fireEvent.click(toggles[0])
    expect(onFocusJob).toHaveBeenLastCalledWith('job-1')

    fireEvent.click(toggles[0])
    expect(onFocusJob).toHaveBeenLastCalledWith(null)
  })
})
