import { formatBytes, formatDurationLong, formatPercentValue } from './format'

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

export { formatDurationLong, formatPercentValue }
