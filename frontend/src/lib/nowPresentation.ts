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

export type NowReliabilityPresentation = {
  label: string
  detail: string
  tone: 'ok' | 'warning' | 'critical'
}

export function presentNowReliability(
  state: NowSnapshot['reliability']['state'],
): NowReliabilityPresentation {
  if (state === 'degraded') {
    return {
      label: 'Degraded',
      detail: 'One or more sources cannot confirm current state.',
      tone: 'critical',
    }
  }
  if (state === 'attention') {
    return {
      label: 'Needs attention',
      detail: 'Current evidence has an actionable reliability signal.',
      tone: 'warning',
    }
  }
  return {
    label: 'Operational',
    detail: 'Available sources agree that the host is within expected state.',
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
    reliability: {
      ...snapshot.reliability,
      state: 'degraded',
    },
    sources: {
      tmux: mark(snapshot.sources.tmux),
      services: mark(snapshot.sources.services),
      metrics: mark(snapshot.sources.metrics),
      runbooks: mark(snapshot.sources.runbooks),
    },
  }
}

export function nowAttentionHiddenCount(attention: NowAttention): number {
  return (
    attention.overflow.approvals +
    attention.overflow.services +
    attention.overflow.runbooks +
    attention.overflow.metrics
  )
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
