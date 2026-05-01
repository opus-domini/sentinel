import { describe, expect, it } from 'vitest'
import type { OpsRunbook, OpsRunbookRun } from '@/types'
import {
  formatRunbookDuration,
  isActiveRunbookJob,
  latestRunbookJob,
  runbookJobDurationMs,
  runbookJobProgress,
  runbookSearchText,
  runbookStatusMeta,
} from './runbookPresentation'

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
    finishedAt: '2026-01-01T10:01:10Z',
    ...overrides,
  }
}

describe('runbookPresentation', () => {
  it('derives status from the latest job', () => {
    const rb = runbook()
    const olderFailure = job({
      id: 'old',
      status: 'failed',
      createdAt: '2026-01-01T09:00:00Z',
    })
    const running = job({
      id: 'new',
      status: 'running',
      completedSteps: 1,
      createdAt: '2026-01-01T11:00:00Z',
      finishedAt: '',
    })

    expect(latestRunbookJob(rb.id, [olderFailure, running])?.id).toBe('new')
    expect(runbookStatusMeta(rb, [olderFailure, running])).toMatchObject({
      label: 'Running',
      tone: 'warning',
    })
  })

  it('marks disabled runbooks before checking job history', () => {
    expect(
      runbookStatusMeta(runbook({ enabled: false }), [
        job({ status: 'succeeded' }),
      ]),
    ).toMatchObject({ label: 'Disabled', tone: 'muted' })
  })

  it('indexes operational fields for sidebar search', () => {
    const text = runbookSearchText(
      runbook({
        parameters: [
          {
            name: 'service',
            label: 'Service name',
            type: 'select',
            default: 'nginx',
            required: true,
            options: ['nginx', 'postgres'],
          },
        ],
        steps: [
          {
            type: 'run',
            title: 'Restart service',
            command: 'systemctl restart {{service}}',
          },
        ],
      }),
    )

    expect(text).toContain('postgres')
    expect(text).toContain('systemctl restart')
  })

  it('formats progress and durations', () => {
    const running = job({
      status: 'running',
      completedSteps: 1,
      totalSteps: 4,
      startedAt: '2026-01-01T10:00:00Z',
      finishedAt: '',
    })

    expect(isActiveRunbookJob(running)).toBe(true)
    expect(runbookJobProgress(running)).toBe(25)
    expect(
      runbookJobDurationMs(running, new Date('2026-01-01T10:02:30Z')),
    ).toBe(150000)
    expect(formatRunbookDuration(150000)).toBe('2m 30s')
    expect(formatRunbookDuration(700)).toBe('700ms')
  })
})
