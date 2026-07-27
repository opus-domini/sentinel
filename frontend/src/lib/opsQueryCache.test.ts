import { describe, expect, it } from 'vitest'

import {
  OPS_METRICS_QUERY_KEY,
  OPS_NOW_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_RUNBOOKS_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  OPS_STORAGE_STATS_QUERY_KEY,
  isOpsWsMessage,
  isNowRelevantEvent,
  metricsCacheValueFromMessage,
  upsertOpsRunbookJob,
} from './opsQueryCache'
import type { OpsHostMetrics, OpsRunbookRun } from '@/types'

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
    expect(OPS_NOW_QUERY_KEY).toEqual(['ops', 'now'])
    expect(OPS_STORAGE_STATS_QUERY_KEY).toEqual(['ops', 'storage-stats'])
  })

  it.each([
    'tmux.sessions.updated',
    'ops.services.updated',
    'ops.overview.updated',
    'ops.posture.updated',
    'ops.job.updated',
  ])('invalidates Now for %s', (type) => {
    expect(isNowRelevantEvent({ type, payload: {} })).toBe(true)
  })

  it('does not invalidate Now for unrelated or malformed messages', () => {
    expect(isNowRelevantEvent({ type: 'ops.schedule.updated', payload: {} })).toBe(false)
    expect(isNowRelevantEvent({ type: 'ops.metrics.updated', payload: {} })).toBe(false)
    expect(isNowRelevantEvent({ payload: {} })).toBe(false)
    expect(isNowRelevantEvent(null)).toBe(false)
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

  it('keeps the HTTP metrics cache shape when applying websocket updates', () => {
    const metrics = { cpuPercent: 85 } as OpsHostMetrics
    const posture = {
      state: 'pressure' as const,
      severity: 'warning' as const,
      warningCount: 1,
      criticalCount: 0,
      observedAt: '2026-07-27T12:00:00Z',
      signals: [
        {
          name: 'cpu' as const,
          severity: 'warning' as const,
          value: 85,
          since: '2026-07-27T11:59:50Z',
        },
      ],
    }
    const value = metricsCacheValueFromMessage({
      type: 'ops.metrics.updated',
      payload: { metrics, posture },
    })

    expect(value).toEqual({ metrics, posture })
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
