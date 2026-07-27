import type {
  ConnectionState,
  NowAttention,
  NowSnapshot,
  NowSource,
  NowSourceStatus,
} from '@/types'

export const NOW_SOURCE_ORDER = ['services', 'metrics', 'runbooks', 'tmux'] as const
export type NowSourceName = (typeof NOW_SOURCE_ORDER)[number]

export const NOW_QUERY_REFRESH_POLICY = {
  retry: false,
  refetchInterval: false,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const

export type NowPosturePresentation = {
  label: string
  detail: string
  tone: 'ok' | 'warning' | 'critical'
}

export function presentNowPosture(state: NowSnapshot['posture']['state']): NowPosturePresentation {
  if (state === 'unknown') {
    return {
      label: 'Unknown',
      detail: 'Services or Metrics cannot confirm the current host posture.',
      tone: 'critical',
    }
  }
  if (state === 'at_risk') {
    return {
      label: 'At risk',
      detail: 'Current service or metric evidence needs operator attention.',
      tone: 'warning',
    }
  }
  return {
    label: 'Healthy',
    detail: 'Current Services and Metrics evidence is within expected state.',
    tone: 'ok',
  }
}

export function nowSourceLabel(source: NowSourceName): string {
  switch (source) {
    case 'services':
      return 'Services'
    case 'metrics':
      return 'Metrics'
    case 'runbooks':
      return 'Runbooks'
    case 'tmux':
      return 'Tmux'
  }
}

export function nowSourceStatusLabel(status: NowSourceStatus): string {
  switch (status) {
    case 'current':
      return 'Current'
    case 'stale':
      return 'Stale'
    case 'not_configured':
      return 'Not configured'
    case 'unavailable':
      return 'Unavailable'
  }
}

export function shouldPresentNowSnapshotAsStale(
  connectionState: ConnectionState,
  refetchFailed: boolean,
): boolean {
  return connectionState === 'disconnected' || connectionState === 'error' || refetchFailed
}

export function markNowCurrentSourcesStale(snapshot: NowSnapshot): NowSnapshot {
  const mark = (source: NowSource): NowSource =>
    source.status === 'current'
      ? {
          ...source,
          status: 'stale',
          message: source.message || 'events_disconnected',
        }
      : source

  return {
    ...snapshot,
    confidence: {
      ...snapshot.confidence,
      state: 'degraded',
      sources: {
        tmux: mark(snapshot.confidence.sources.tmux),
        services: mark(snapshot.confidence.sources.services),
        metrics: mark(snapshot.confidence.sources.metrics),
        runbooks: mark(snapshot.confidence.sources.runbooks),
      },
    },
    posture: {
      ...snapshot.posture,
      state: 'unknown',
    },
  }
}

export function nowAttentionHiddenCount(attention: NowAttention): number {
  return attention.overflow.approvals + attention.overflow.services + attention.overflow.metrics
}

export function nowRunbookSearch(runbookId: string, runId: string) {
  return { runbook: runbookId, job: runId }
}

export function nowServiceSearch(serviceName: string, panel: 'status' | 'logs' = 'status') {
  return { service: serviceName, panel }
}

export function nowTmuxSearch(session: string) {
  return { session }
}
