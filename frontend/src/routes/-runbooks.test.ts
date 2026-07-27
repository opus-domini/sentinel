import { describe, expect, it, vi } from 'vitest'
import type { OpsRunbook, OpsRunbookRun } from '@/types'
import { resolveRunbooksDeepLink } from './runbooks'

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: () => null,
}))

const runbook: OpsRunbook = {
  id: 'runbook-1',
  name: 'Recovery',
  description: '',
  enabled: true,
  steps: [{ type: 'run', title: 'Recover', command: 'true' }],
  createdAt: '2026-07-27T10:00:00Z',
  updatedAt: '2026-07-27T10:00:00Z',
}

const job: OpsRunbookRun = {
  id: 'job-1',
  runbookId: 'runbook-1',
  runbookName: 'Recovery',
  status: 'succeeded',
  totalSteps: 1,
  completedSteps: 1,
  currentStep: '',
  error: '',
  stepResults: [],
  createdAt: '2026-07-27T10:00:00Z',
}

describe('Runbooks route deep-link resolution', () => {
  it('treats the execution as authoritative and resolves its current definition', () => {
    expect(resolveRunbooksDeepLink({ runbook: 'wrong', job: 'job-1' }, [runbook], job)).toEqual({
      kind: 'execution',
      key: 'job:job-1:runbook-1',
      jobId: 'job-1',
      selectedRunbookId: 'runbook-1',
    })
  })

  it('keeps a receipt standalone when its definition was deleted', () => {
    expect(resolveRunbooksDeepLink({ job: 'job-1' }, [], job)).toEqual({
      kind: 'execution',
      key: 'job:job-1:standalone',
      jobId: 'job-1',
      selectedRunbookId: null,
    })
  })

  it('rejects missing definitions and missing execution receipts', () => {
    expect(resolveRunbooksDeepLink({ runbook: 'missing' }, [], null)).toEqual({
      kind: 'invalid',
    })
    expect(resolveRunbooksDeepLink({ job: 'missing' }, [runbook], null)).toEqual({
      kind: 'invalid',
    })
  })
})
