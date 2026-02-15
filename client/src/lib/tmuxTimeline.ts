import type { TimelineEvent } from '@/types'

type TimelineEventFilter = {
  session?: string
  query?: string
  severity?: string
  eventType?: string
  limit?: number
}

export type TimelineEventLocation = {
  label: string
  fallback: string
}

function metadataString(
  metadata: Record<string, unknown> | null | undefined,
  keys: Array<string>,
): string {
  if (metadata == null) return ''
  for (const key of keys) {
    const value = metadata[key]
    if (typeof value !== 'string') continue
    const normalized = value.trim()
    if (normalized !== '') return normalized
  }
  return ''
}

function normalizeWindowFallback(windowIndex: number): string {
  if (!Number.isFinite(windowIndex) || windowIndex < 0) return '?'
  return String(Math.trunc(windowIndex))
}

function normalizePaneFallback(paneID: string): string {
  const normalized = paneID.trim()
  if (normalized === '') return '?'
  return normalized
}

export function formatTimelineEventLocation(
  event: TimelineEvent,
): TimelineEventLocation {
  const metadata =
    event.metadata != null && typeof event.metadata === 'object'
      ? event.metadata
      : null
  const session = event.session.trim() !== '' ? event.session.trim() : '?'
  const windowName = metadataString(metadata, ['windowName'])
  const paneName = metadataString(metadata, ['paneTitle', 'title'])

  const windowLabel =
    windowName !== ''
      ? windowName
      : `#${normalizeWindowFallback(event.windowIndex)}`
  const paneLabel =
    paneName !== '' ? paneName : normalizePaneFallback(event.paneId)

  return {
    label: `${session} > ${windowLabel} > ${paneLabel}`,
    fallback: `${session}:${normalizeWindowFallback(event.windowIndex)}:${normalizePaneFallback(event.paneId)}`,
  }
}

export function buildTimelineQueryString(filter: TimelineEventFilter): string {
  const params = new URLSearchParams()

  const session = (filter.session ?? '').trim()
  if (session !== '') {
    params.set('session', session)
  }

  const query = (filter.query ?? '').trim()
  if (query !== '') {
    params.set('q', query)
  }

  const severity = (filter.severity ?? '').trim().toLowerCase()
  if (severity !== '' && severity !== 'all') {
    params.set('severity', severity)
  }

  const eventType = (filter.eventType ?? '').trim()
  if (eventType !== '' && eventType !== 'all') {
    params.set('eventType', eventType)
  }

  const limit = Number.isFinite(filter.limit)
    ? Math.trunc(filter.limit ?? 0)
    : 0
  if (limit > 0) {
    params.set('limit', String(limit))
  }

  const encoded = params.toString()
  if (encoded === '') {
    return ''
  }
  return `?${encoded}`
}

export function shouldRefreshTimelineFromEvent(
  sessions: unknown,
  trackedSession: string,
): boolean {
  const scope = trackedSession.trim()
  if (!Array.isArray(sessions)) {
    return false
  }
  if (scope === '' || scope === 'all') {
    return sessions.length > 0
  }
  return sessions.some(
    (item) => typeof item === 'string' && item.trim() === scope,
  )
}
