export function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.trunc(totalSeconds))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  )
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

export function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return fallback
}

export function formatTimeAgo(iso: string): string {
  const diff = Math.max(
    0,
    Math.trunc((Date.now() - new Date(iso).getTime()) / 1000),
  )
  if (diff < 60) return `${diff}s ago`
  const minutes = Math.floor(diff / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ago`
}

export function browsedServiceDot(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') return 'bg-emerald-500'
  if (s === 'failed') return 'bg-red-500'
  return 'bg-muted-foreground/50'
}
