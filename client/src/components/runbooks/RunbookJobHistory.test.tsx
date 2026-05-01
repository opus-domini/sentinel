// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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
})
