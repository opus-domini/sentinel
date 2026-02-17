import { describe, expect, it } from 'vitest'

import {
  OPS_ALERTS_QUERY_KEY,
  OPS_CONFIG_QUERY_KEY,
  OPS_GUARDRAILS_QUERY_KEY,
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_RUNBOOKS_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  OPS_STORAGE_STATS_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
  upsertOpsRunbookJob,
} from './opsQueryCache'
import type { OpsRunbookRun, OpsTimelineEvent } from '@/types'

function buildTimelineEvent(id: number): OpsTimelineEvent {
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
    expect(OPS_CONFIG_QUERY_KEY).toEqual(['ops', 'config'])
    expect(OPS_GUARDRAILS_QUERY_KEY).toEqual(['ops', 'guardrails'])
    expect(OPS_STORAGE_STATS_QUERY_KEY).toEqual(['ops', 'storage-stats'])
  })

  it('builds normalized timeline query key', () => {
    expect(opsTimelineQueryKey('  service  ', ' WARN ')).toEqual([
      'ops',
      'timeline',
      'service',
      'warn',
    ])
  })

  it('prepends timeline event and deduplicates by id', () => {
    const first = buildTimelineEvent(1)
    const second = buildTimelineEvent(2)
    const updatedFirst = { ...first, message: 'updated' }

    expect(
      prependOpsTimelineEvent([first], second).map((item) => item.id),
    ).toEqual([2, 1])
    expect(
      prependOpsTimelineEvent([first, second], updatedFirst).map(
        (item) => item.message,
      ),
    ).toEqual(['updated', 'event-2'])
  })

  it('upserts runbook jobs and keeps latest first', () => {
    const first = buildJob('a')
    const second = buildJob('b')
    const updatedFirst = { ...first, status: 'succeeded' }

    expect(upsertOpsRunbookJob([first], second).map((item) => item.id)).toEqual(
      ['b', 'a'],
    )
    expect(
      upsertOpsRunbookJob([first, second], updatedFirst).map(
        (item) => item.status,
      ),
    ).toEqual(['succeeded', 'queued'])
  })
})
