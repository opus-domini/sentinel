export function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.trunc(totalSeconds))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  const precision = size >= 100 || index === 0 ? 0 : 1
  return `${size.toFixed(precision)} ${units[index]}`
}

export function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return fallback
}

export function browsedServiceDot(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') return 'bg-emerald-500'
  if (s === 'failed') return 'bg-red-500'
  return 'bg-muted-foreground/50'
}
