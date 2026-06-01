import { describe, expect, it } from 'vitest'

import {
  OPS_ALERTS_QUERY_KEY,
  OPS_ACTIVITY_VISIBLE_LIMIT,
  OPS_GUARDRAILS_QUERY_KEY,
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_RUNBOOKS_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  OPS_STORAGE_STATS_QUERY_KEY,
  isOpsWsMessage,
  opsActivityQueryKey,
  prependOpsActivityEvent,
  upsertOpsRunbookJob,
} from './opsQueryCache'
import type { OpsActivityEvent, OpsRunbookRun } from '@/types'

function buildActivityEvent(id: number): OpsActivityEvent {
  return {
    id,
    source: 'ops',
    eventType: 'service',
    severity: 'info',
    resource: 'svc',
    message: `event-${id}`,
    details: '',
    metadata: '',
    createdAt: '2026-01-01T00:00:00Z',
  }
}

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
    expect(OPS_ALERTS_QUERY_KEY).toEqual(['ops', 'alerts'])
    expect(OPS_RUNBOOKS_QUERY_KEY).toEqual(['ops', 'runbooks'])
    expect(OPS_METRICS_QUERY_KEY).toEqual(['ops', 'metrics'])
    expect(OPS_GUARDRAILS_QUERY_KEY).toEqual(['ops', 'guardrails'])
    expect(OPS_STORAGE_STATS_QUERY_KEY).toEqual(['ops', 'storage-stats'])
  })

  it('builds normalized activity query key', () => {
    expect(opsActivityQueryKey('  service  ', ' WARN ')).toEqual([
      'ops',
      'activity',
      'service',
      'warn',
    ])
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

  it('prepends activity event and deduplicates by id', () => {
    const first = buildActivityEvent(1)
    const second = buildActivityEvent(2)
    const updatedFirst = { ...first, message: 'updated' }

    expect(prependOpsActivityEvent([first], second).map((item) => item.id)).toEqual([2, 1])
    expect(
      prependOpsActivityEvent([first, second], updatedFirst).map((item) => item.message),
    ).toEqual(['updated', 'event-2'])
  })

  it('caps activity events at the visible limit', () => {
    const events = Array.from({ length: OPS_ACTIVITY_VISIBLE_LIMIT }, (_, index) =>
      buildActivityEvent(index + 1),
    )
    const next = buildActivityEvent(999)

    const result = prependOpsActivityEvent(events, next)

    expect(result).toHaveLength(OPS_ACTIVITY_VISIBLE_LIMIT)
    expect(result[0].id).toBe(999)
    expect(result.at(-1)?.id).toBe(OPS_ACTIVITY_VISIBLE_LIMIT - 1)
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
