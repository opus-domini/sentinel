import type { MetricPostureSignal } from '@/types'

export type ServicesSearch = {
  service?: string
  panel?: 'status' | 'logs'
  since?: string
}

export type RunbooksSearch = {
  runbook?: string
  job?: string
}

export type TmuxSearch = {
  session?: string
}

export type MetricsSearch = {
  signal?: MetricPostureSignal['name']
  focusAt?: string
}

const METRIC_SIGNALS = new Set<MetricPostureSignal['name']>([
  'cpu',
  'memory',
  'rootDisk',
  'inodes',
  'swap',
  'cpuPressure',
  'memoryPressure',
  'ioPressure',
])

function optionalSearchText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed === '' ? undefined : trimmed
}

function optionalRFC3339(value: unknown): string | undefined {
  const text = optionalSearchText(value)
  if (text == null) return undefined
  const match =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|([+-])(\d{2}):(\d{2}))$/.exec(
      text,
    )
  if (match == null) return undefined

  const [, year, month, day, hour, minute, second, , offsetHour, offsetMinute] = match
  const numericYear = Number(year)
  const numericMonth = Number(month)
  const numericDay = Number(day)
  const validDay =
    numericMonth >= 1 &&
    numericMonth <= 12 &&
    numericDay >= 1 &&
    numericDay <= new Date(Date.UTC(numericYear, numericMonth, 0)).getUTCDate()
  const validTime =
    Number(hour) <= 23 &&
    Number(minute) <= 59 &&
    Number(second) <= 59 &&
    (offsetHour == null ||
      (Number(offsetHour) <= 23 && offsetMinute != null && Number(offsetMinute) <= 59))

  return validDay && validTime && !Number.isNaN(Date.parse(text)) ? text : undefined
}

export function parseServicesSearch(search: Record<string, unknown>): ServicesSearch {
  const service = optionalSearchText(search.service)
  const panel = search.panel === 'status' || search.panel === 'logs' ? search.panel : undefined
  if (service == null || panel == null) return {}
  if (panel === 'status') return { service, panel }
  const since = optionalRFC3339(search.since)
  return since == null ? { service, panel } : { service, panel, since }
}

export function parseRunbooksSearch(search: Record<string, unknown>): RunbooksSearch {
  const runbook = optionalSearchText(search.runbook)
  const job = optionalSearchText(search.job)
  if (runbook == null && job == null) return {}
  return { runbook, job }
}

export function parseTmuxSearch(search: Record<string, unknown>): TmuxSearch {
  const session = optionalSearchText(search.session)
  return session == null ? {} : { session }
}

export function parseMetricsSearch(search: Record<string, unknown>): MetricsSearch {
  const signal =
    typeof search.signal === 'string' &&
    METRIC_SIGNALS.has(search.signal as MetricPostureSignal['name'])
      ? (search.signal as MetricPostureSignal['name'])
      : undefined
  if (signal == null) return {}
  const focusAt = optionalRFC3339(search.focusAt)
  return focusAt == null ? { signal } : { signal, focusAt }
}

export function runbookDefinitionSearch(runbook: string): RunbooksSearch {
  return { runbook }
}

export function runbookExecutionSearch(job: string): RunbooksSearch {
  return { job }
}

export function serviceStatusSearch(service: string): ServicesSearch {
  return { service, panel: 'status' }
}

export function serviceLogsSearch(service: string, since?: string): ServicesSearch {
  const canonicalSince = optionalRFC3339(since)
  return canonicalSince == null
    ? { service, panel: 'logs' }
    : { service, panel: 'logs', since: canonicalSince }
}

export function metricsSignalSearch(
  signal: MetricPostureSignal['name'],
  focusAt?: string,
): MetricsSearch {
  const canonicalFocusAt = optionalRFC3339(focusAt)
  return canonicalFocusAt == null ? { signal } : { signal, focusAt: canonicalFocusAt }
}

export function tmuxSessionSearch(session: string): TmuxSearch {
  return { session }
}
