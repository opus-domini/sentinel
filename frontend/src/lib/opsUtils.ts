export { formatBytes, formatUptime } from './format'

export function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return fallback
}

export function browsedServiceDot(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') return 'bg-ok'
  if (s === 'failed') return 'bg-destructive'
  return 'bg-muted-foreground/50'
}
