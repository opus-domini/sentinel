import { formatBytes } from './opsUtils'

export type MetricSeverity = 'ok' | 'warn' | 'critical' | 'unknown'

export function percentSeverity(value: number, warn: number, critical: number): MetricSeverity {
  if (!Number.isFinite(value) || value < 0) return 'unknown'
  if (value >= critical) return 'critical'
  if (value >= warn) return 'warn'
  return 'ok'
}

export function pressureSeverity(value: number): MetricSeverity {
  if (!Number.isFinite(value) || value < 0) return 'unknown'
  if (value >= 10) return 'critical'
  if (value >= 2) return 'warn'
  return 'ok'
}

export function formatPercentValue(value: number, digits = 1): string {
  if (!Number.isFinite(value) || value < 0) return '-'
  return `${value.toFixed(digits)}%`
}

export function computeByteRate(samples: Array<number>, timestamps: Array<number>): number {
  if (samples.length < 2 || timestamps.length < 2) return 0

  const currentIndex = samples.length - 1
  const previousIndex = currentIndex - 1
  const deltaBytes = samples[currentIndex] - samples[previousIndex]
  const deltaMs = timestamps[currentIndex] - timestamps[previousIndex]
  if (deltaBytes <= 0 || deltaMs <= 0) return 0
  return deltaBytes / (deltaMs / 1000)
}

export function formatByteRate(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return '0 B/s'
  return `${formatBytes(bytesPerSecond)}/s`
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
