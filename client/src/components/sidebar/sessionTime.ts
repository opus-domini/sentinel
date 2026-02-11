export function formatRelativeTime(value: string): string {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  const now = Date.now()
  const diffMs = now - date.getTime()
  if (diffMs < 60_000) {
    return 'now'
  }

  const minutes = Math.floor(diffMs / 60_000)
  if (minutes < 60) {
    return `${minutes}m`
  }

  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}h`
  }

  const days = Math.floor(hours / 24)
  if (days < 7) {
    return `${days}d`
  }

  const weeks = Math.floor(days / 7)
  if (weeks < 5) {
    return `${weeks}w`
  }

  const months = Math.floor(days / 30)
  if (months < 12) {
    return `${months}mo`
  }

  const years = Math.floor(days / 365)
  return `${years}y`
}

export function formatTimestamp(value: string): string {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return (
    date.toLocaleDateString() +
    ' ' +
    date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  )
}
