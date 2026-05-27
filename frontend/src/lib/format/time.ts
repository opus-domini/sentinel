function resolveLocale(locale: string): string | undefined {
  return locale || undefined
}

function parseDate(value: string | undefined | null): Date | null {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatDateTime(value: string, timezone: string, locale = ''): string {
  const date = parseDate(value)
  if (!date) return '-'
  try {
    return new Intl.DateTimeFormat(resolveLocale(locale), {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      timeZone: timezone,
    }).format(date)
  } catch {
    return date.toLocaleString()
  }
}

export function formatDateTimeShort(value: string, timezone: string, locale = ''): string {
  const date = parseDate(value)
  if (!date) return '-'
  try {
    return new Intl.DateTimeFormat(resolveLocale(locale), {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      timeZone: timezone,
    }).format(date)
  } catch {
    return date.toLocaleString()
  }
}

export function formatRelativeTime(value: string, timezone: string, locale = ''): string {
  const parsed = Date.parse(value)
  if (Number.isNaN(parsed)) return value
  const diffMin = Math.floor((Date.now() - parsed) / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  return formatDateTimeShort(value, timezone, locale)
}

export function formatCompactRelativeTime(value: string): string {
  const date = parseDate(value)
  if (!date) return '-'

  const diffMs = Date.now() - date.getTime()
  if (diffMs < 60_000) return 'now'

  const minutes = Math.floor(diffMs / 60_000)
  if (minutes < 60) return `${minutes}m`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`

  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d`

  const weeks = Math.floor(days / 7)
  if (weeks < 5) return `${weeks}w`

  const months = Math.floor(days / 30)
  if (months < 12) return `${months}mo`

  return `${Math.floor(days / 365)}y`
}

export function formatTimestamp(value: string, timezone = '', locale = ''): string {
  const date = parseDate(value)
  if (!date) return '-'
  if (timezone) {
    try {
      const loc = resolveLocale(locale)
      const datePart = new Intl.DateTimeFormat(loc, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        timeZone: timezone,
      }).format(date)
      const timePart = new Intl.DateTimeFormat(loc, {
        hour: '2-digit',
        minute: '2-digit',
        timeZone: timezone,
      }).format(date)
      return `${datePart} ${timePart}`
    } catch {
      // fall through to browser default formatting
    }
  }
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })}`
}

export function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.trunc(totalSeconds))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

export function formatDurationLong(totalSeconds: number): string {
  const seconds = Math.max(0, Math.trunc(totalSeconds))
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

export function formatElapsedRelativeTime(timestamp: number): string {
  const diff = Math.round((Date.now() - timestamp) / 1000)
  if (diff < 10) return 'just now'
  if (diff < 60) return `${diff}s ago`
  const minutes = Math.floor(diff / 60)
  const seconds = diff % 60
  if (minutes < 60) return seconds > 0 ? `${minutes}m ${seconds}s ago` : `${minutes}m ago`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m ago`
}
