import { cn } from '@/lib/utils'

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

export function browsedServiceDot(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') return 'bg-emerald-500'
  if (s === 'failed') return 'bg-red-500'
  return 'bg-muted-foreground/50'
}

export function opsTabButtonClass(active: boolean): string {
  return cn(
    'inline-flex cursor-pointer items-center gap-1 rounded-md border px-2.5 py-1 text-[11px] font-medium transition-colors',
    active
      ? 'border-primary/40 bg-primary/15 text-primary-text-bright'
      : 'border-transparent text-muted-foreground hover:border-border hover:bg-surface-overlay hover:text-foreground',
  )
}
