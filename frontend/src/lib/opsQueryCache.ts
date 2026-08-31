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
  'ops.posture.updated',
  'ops.job.updated',
  'ops.runbooks.updated',
])

// Discriminants the guard below actually proves. Kept as OpsWsMessage['type']
// so a union member that is added here without a matching entry, or an entry
// that no longer exists in the union, fails to compile.
const OPS_WS_MESSAGE_TYPES: ReadonlySet<string> = new Set<OpsWsMessage['type']>([
  'events.ready',
  'ops.overview.updated',
  'ops.services.updated',
  'ops.metrics.updated',
  'ops.posture.updated',
  'ops.runbooks.updated',
  'ops.job.updated',
  'ops.schedule.updated',
  'tmux.sessions.updated',
  'tmux.inspector.updated',
  'tmux.activity.updated',
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function isOpsWsMessage(msg: unknown): msg is OpsWsMessage {
  if (!isRecord(msg)) return false
  if (typeof msg.type !== 'string') return false
  if (!OPS_WS_MESSAGE_TYPES.has(msg.type)) return false
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
