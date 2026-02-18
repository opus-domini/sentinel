import type { OpsRunbookRun, OpsTimelineEvent } from '@/types'

export const OPS_BROWSE_QUERY_KEY = ['ops', 'browse'] as const
export const OPS_OVERVIEW_QUERY_KEY = ['ops', 'overview'] as const
export const OPS_SERVICES_QUERY_KEY = ['ops', 'services'] as const
export const OPS_ALERTS_QUERY_KEY = ['ops', 'alerts'] as const
export const OPS_RUNBOOKS_QUERY_KEY = ['ops', 'runbooks'] as const
export const OPS_METRICS_QUERY_KEY = ['ops', 'metrics'] as const
export const OPS_GUARDRAILS_QUERY_KEY = ['ops', 'guardrails'] as const
export const OPS_STORAGE_STATS_QUERY_KEY = ['ops', 'storage-stats'] as const

export function opsTimelineQueryKey(query: string, severity: string) {
  return [
    'ops',
    'timeline',
    query.trim(),
    severity.trim().toLowerCase(),
  ] as const
}

export function prependOpsTimelineEvent(
  events: Array<OpsTimelineEvent>,
  next: OpsTimelineEvent,
): Array<OpsTimelineEvent> {
  return [next, ...events.filter((item) => item.id !== next.id)]
}

export function upsertOpsRunbookJob(
  jobs: Array<OpsRunbookRun>,
  next: OpsRunbookRun,
): Array<OpsRunbookRun> {
  return [next, ...jobs.filter((item) => item.id !== next.id)]
}
