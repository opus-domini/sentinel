import { describe, expect, it } from 'vitest'
import type { NowSnapshot } from '@/types'
import {
  NOW_QUERY_REFRESH_POLICY,
  markNowCurrentSourcesStale,
  nowAttentionHiddenCount,
  nowRunbookSearch,
  nowServiceSearch,
  nowTmuxSearch,
  shouldPresentNowSnapshotAsStale,
} from './nowPresentation'

const snapshot: NowSnapshot = {
  generatedAt: '2026-07-27T12:00:00Z',
  reliability: {
    state: 'normal',
    services: { tracked: 1, running: 1, failed: 0, inactive: 0, unknown: 0 },
    metrics: {
      state: 'normal',
      severity: 'ok',
      warningCount: 0,
      criticalCount: 0,
      signals: [],
    },
  },
  attention: {
    total: 0,
    visible: [],
    overflow: { approvals: 0, services: 0, runbooks: 0, metrics: 0 },
  },
  inProgress: { runs: [], sessions: [] },
  sources: {
    services: { status: 'current', checkedAt: '2026-07-27T12:00:00Z' },
    metrics: { status: 'unavailable', checkedAt: '2026-07-27T12:00:00Z' },
    runbooks: { status: 'current', checkedAt: '2026-07-27T12:00:00Z' },
    tmux: { status: 'not_configured', checkedAt: '2026-07-27T12:00:00Z' },
  },
}

describe('Now presentation', () => {
  it('marks only current sources stale and degrades reliability', () => {
    const stale = markNowCurrentSourcesStale(snapshot)
    expect(stale.reliability.state).toBe('degraded')
    expect(stale.sources.services.status).toBe('stale')
    expect(stale.sources.runbooks.status).toBe('stale')
    expect(stale.sources.metrics.status).toBe('unavailable')
    expect(stale.sources.tmux.status).toBe('not_configured')
    expect(snapshot.sources.services.status).toBe('current')
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

  it('counts hidden items and builds canonical owner searches', () => {
    expect(
      nowAttentionHiddenCount({
        total: 9,
        visible: [],
        overflow: { approvals: 1, services: 2, runbooks: 0, metrics: 1 },
      }),
    ).toBe(4)
    expect(nowServiceSearch('sentinel')).toEqual({ service: 'sentinel', panel: 'status' })
    expect(nowRunbookSearch('rb', 'run')).toEqual({ runbook: 'rb', job: 'run' })
    expect(nowTmuxSearch('dev')).toEqual({ session: 'dev' })
  })
})
