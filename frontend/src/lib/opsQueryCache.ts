import type { OpsMetricsResponse, OpsRunbookRun, OpsWsMessage } from '@/types'

export const OPS_BROWSE_QUERY_KEY = ['ops', 'browse'] as const
export const OPS_OVERVIEW_QUERY_KEY = ['ops', 'overview'] as const
export const OPS_SERVICES_QUERY_KEY = ['ops', 'services'] as const
export const OPS_RUNBOOKS_QUERY_KEY = ['ops', 'runbooks'] as const
export const OPS_METRICS_QUERY_KEY = ['ops', 'metrics'] as const
export const OPS_NOW_QUERY_KEY = ['ops', 'now'] as const
export const OPS_STORAGE_STATS_QUERY_KEY = ['ops', 'storage-stats'] as const

const NOW_RELEVANT_EVENT_TYPES = new Set([
  'tmux.sessions.updated',
  'ops.services.updated',
  'ops.overview.updated',
  'ops.posture.updated',
  'ops.job.updated',
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function isOpsWsMessage(msg: unknown): msg is OpsWsMessage {
  if (!isRecord(msg)) return false
  if (typeof msg.type !== 'string') return false
  return isRecord(msg.payload)
}

export function upsertOpsRunbookJob(
  jobs: Array<OpsRunbookRun>,
  next: OpsRunbookRun,
): Array<OpsRunbookRun> {
  return [next, ...jobs.filter((item) => item.id !== next.id)]
}

export function metricsCacheValueFromMessage(message: OpsWsMessage): OpsMetricsResponse | null {
  if (message.type !== 'ops.metrics.updated') return null
  return message.payload
}

export function isNowRelevantEvent(message: unknown): boolean {
  if (!isRecord(message) || typeof message.type !== 'string') return false
  return NOW_RELEVANT_EVENT_TYPES.has(message.type)
}
