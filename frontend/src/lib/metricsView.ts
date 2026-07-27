import { formatBytes, formatDurationLong, formatPercentValue } from './format'
import type { MetricPosture } from '@/types'

export type MetricSeverity = 'ok' | 'warn' | 'critical' | 'unknown'

export type MetricPosturePresentation = {
  severity: MetricSeverity
  label: string
  detail: string
}

export function presentMetricPosture(posture: MetricPosture | null): MetricPosturePresentation {
  if (posture == null) {
    return { severity: 'unknown', label: 'Waiting', detail: 'no posture received' }
  }
  if (posture.state === 'unavailable') {
    return { severity: 'unknown', label: 'Unavailable', detail: 'no evaluable key signals' }
  }
  if (posture.severity === 'critical') {
    return {
      severity: 'critical',
      label: 'Critical',
      detail: formatSignalCounts(posture.criticalCount, posture.warningCount),
    }
  }
  if (posture.severity === 'warning') {
    return {
      severity: 'warn',
      label: 'Attention',
      detail: formatSignalCounts(0, posture.warningCount),
    }
  }
  return { severity: 'ok', label: 'Nominal', detail: 'all available key signals green' }
}

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

function formatSignalCounts(critical: number, warning: number): string {
  const parts: Array<string> = []
  if (critical > 0) {
    parts.push(`${critical} critical signal${critical === 1 ? '' : 's'}`)
  }
  if (warning > 0) {
    parts.push(`${warning} warning signal${warning === 1 ? '' : 's'}`)
  }
  return parts.join(' · ')
}

export { formatDurationLong, formatPercentValue }
