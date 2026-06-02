import { describe, expect, it } from 'vitest'

import {
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_RUNBOOKS_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  OPS_STORAGE_STATS_QUERY_KEY,
  isOpsWsMessage,
  upsertOpsRunbookJob,
} from './opsQueryCache'
import type { OpsRunbookRun } from '@/types'

function buildJob(id: string): OpsRunbookRun {
  return {
    id,
    runbookId: 'rb',
    runbookName: 'Runbook',
    status: 'queued',
    totalSteps: 3,
    completedSteps: 0,
    currentStep: 'init',
    error: '',
    stepResults: [],
    createdAt: '2026-01-01T00:00:00Z',
  }
}

describe('opsQueryCache', () => {
  it('exposes stable base query keys', () => {
    expect(OPS_OVERVIEW_QUERY_KEY).toEqual(['ops', 'overview'])
    expect(OPS_SERVICES_QUERY_KEY).toEqual(['ops', 'services'])
    expect(OPS_RUNBOOKS_QUERY_KEY).toEqual(['ops', 'runbooks'])
    expect(OPS_METRICS_QUERY_KEY).toEqual(['ops', 'metrics'])
    expect(OPS_STORAGE_STATS_QUERY_KEY).toEqual(['ops', 'storage-stats'])
  })

  it('validates ops websocket message shape', () => {
    expect(
      isOpsWsMessage({
        type: 'ops.overview.updated',
        payload: { overview: {} },
      }),
    ).toBe(true)
    expect(isOpsWsMessage({ type: 'ops.overview.updated' })).toBe(false)
    expect(isOpsWsMessage({ payload: { overview: {} } })).toBe(false)
    expect(isOpsWsMessage(null)).toBe(false)
  })

  it('upserts runbook jobs and keeps latest first', () => {
    const first = buildJob('a')
    const second = buildJob('b')
    const updatedFirst = { ...first, status: 'succeeded' }

    expect(upsertOpsRunbookJob([first], second).map((item) => item.id)).toEqual(['b', 'a'])
    expect(upsertOpsRunbookJob([first, second], updatedFirst).map((item) => item.status)).toEqual([
      'succeeded',
      'queued',
    ])
  })
})
