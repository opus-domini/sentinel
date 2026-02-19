import type { OpsActivityEvent, OpsRunbookRun, OpsWsMessage } from '@/types'

export const OPS_BROWSE_QUERY_KEY = ['ops', 'browse'] as const
export const OPS_OVERVIEW_QUERY_KEY = ['ops', 'overview'] as const
export const OPS_SERVICES_QUERY_KEY = ['ops', 'services'] as const
export const OPS_ALERTS_QUERY_KEY = ['ops', 'alerts'] as const
export const OPS_RUNBOOKS_QUERY_KEY = ['ops', 'runbooks'] as const
export const OPS_METRICS_QUERY_KEY = ['ops', 'metrics'] as const
export const OPS_GUARDRAILS_QUERY_KEY = ['ops', 'guardrails'] as const
export const OPS_GUARDRAILS_AUDIT_QUERY_KEY = [
  'ops',
  'guardrails-audit',
] as const
export const OPS_STORAGE_STATS_QUERY_KEY = ['ops', 'storage-stats'] as const

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function isOpsWsMessage(msg: unknown): msg is OpsWsMessage {
  if (!isRecord(msg)) return false
  if (typeof msg.type !== 'string') return false
  return isRecord(msg.payload)
}

export function opsActivityQueryKey(query: string, severity: string) {
  return [
    'ops',
    'activity',
    query.trim(),
    severity.trim().toLowerCase(),
  ] as const
}

export function prependOpsActivityEvent(
  events: Array<OpsActivityEvent>,
  next: OpsActivityEvent,
): Array<OpsActivityEvent> {
  return [next, ...events.filter((item) => item.id !== next.id)]
}

export function upsertOpsRunbookJob(
  jobs: Array<OpsRunbookRun>,
  next: OpsRunbookRun,
): Array<OpsRunbookRun> {
  return [next, ...jobs.filter((item) => item.id !== next.id)]
}
