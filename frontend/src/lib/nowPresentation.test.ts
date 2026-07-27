import { describe, expect, it } from 'vitest'
import type { NowSnapshot } from '@/types'
import {
  NOW_QUERY_REFRESH_POLICY,
  markNowCurrentSourcesStale,
  nowAttentionHiddenCount,
  shouldPresentNowSnapshotAsStale,
} from './nowPresentation'

const snapshot: NowSnapshot = {
  generatedAt: '2026-07-27T12:00:00Z',
  confidence: {
    state: 'current',
    sources: {
      services: { status: 'current', observedAt: '2026-07-27T12:00:00Z' },
      metrics: { status: 'unavailable', observedAt: '2026-07-27T12:00:00Z' },
      runbooks: { status: 'current', observedAt: '2026-07-27T12:00:00Z' },
      tmux: { status: 'not_configured', observedAt: '2026-07-27T12:00:00Z' },
    },
  },
  posture: {
    state: 'healthy',
    services: { tracked: 1, running: 1, failed: 0, inactive: 0, unknown: 0 },
    metrics: {
      state: 'normal',
      severity: 'ok',
      warningCount: 0,
      criticalCount: 0,
      signals: [],
      observedAt: '2026-07-27T12:00:00Z',
    },
  },
  attention: {
    total: 0,
    visible: [],
    overflow: { approvals: 0, services: 0, metrics: 0 },
  },
  inProgress: { runs: [], sessions: [] },
}

describe('Now presentation', () => {
  it('marks only current sources stale and separates confidence from posture', () => {
    const stale = markNowCurrentSourcesStale(snapshot)
    expect(stale.confidence.state).toBe('degraded')
    expect(stale.posture.state).toBe('unknown')
    expect(stale.confidence.sources.services.status).toBe('stale')
    expect(stale.confidence.sources.runbooks.status).toBe('stale')
    expect(stale.confidence.sources.metrics.status).toBe('unavailable')
    expect(stale.confidence.sources.tmux.status).toBe('not_configured')
    expect(stale.posture.services).toEqual(snapshot.posture.services)
    expect(stale.posture.metrics).toEqual(snapshot.posture.metrics)
    expect(snapshot.confidence.sources.services.status).toBe('current')
  })

  it('uses disconnect and refetch failure as stale signals', () => {
    expect(shouldPresentNowSnapshotAsStale('connected', false)).toBe(false)
    expect(shouldPresentNowSnapshotAsStale('connecting', false)).toBe(false)
    expect(shouldPresentNowSnapshotAsStale('disconnected', false)).toBe(true)
    expect(shouldPresentNowSnapshotAsStale('error', false)).toBe(true)
    expect(shouldPresentNowSnapshotAsStale('connected', true)).toBe(true)
  })

  it('refreshes from events and explicit resync instead of polling', () => {
    expect(NOW_QUERY_REFRESH_POLICY).toEqual({
      retry: false,
      refetchInterval: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    })
  })

  it('counts hidden attention items', () => {
    expect(
      nowAttentionHiddenCount({
        total: 9,
        visible: [],
        overflow: { approvals: 1, services: 2, metrics: 1 },
      }),
    ).toBe(4)
  })
})
