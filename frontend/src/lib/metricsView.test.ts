import { describe, expect, it } from 'vitest'

import {
  computeByteRate,
  formatByteRate,
  formatDurationLong,
  formatPercentValue,
  percentSeverity,
  presentMetricPosture,
  pressureSeverity,
} from './metricsView'

describe('metricsView', () => {
  it('classifies percentage severity with unknown values', () => {
    expect(percentSeverity(42, 80, 90)).toBe('ok')
    expect(percentSeverity(81, 80, 90)).toBe('warn')
    expect(percentSeverity(95, 80, 90)).toBe('critical')
    expect(percentSeverity(-1, 80, 90)).toBe('unknown')
  })

  it('classifies Linux pressure stall averages', () => {
    expect(pressureSeverity(0.5)).toBe('ok')
    expect(pressureSeverity(2)).toBe('warn')
    expect(pressureSeverity(10)).toBe('critical')
    expect(pressureSeverity(-1)).toBe('unknown')
  })

  it('presents the canonical backend posture without recomputing host thresholds', () => {
    expect(
      presentMetricPosture({
        state: 'pressure',
        severity: 'critical',
        warningCount: 1,
        criticalCount: 2,
        signals: [
          { name: 'cpu', severity: 'warning', value: 85 },
          { name: 'memory', severity: 'critical', value: 95 },
          { name: 'ioPressure', severity: 'critical', value: 12 },
        ],
      }),
    ).toEqual({
      severity: 'critical',
      label: 'Critical',
      detail: '2 critical signals · 1 warning signal',
    })
    expect(
      presentMetricPosture({
        state: 'unavailable',
        severity: 'unknown',
        warningCount: 0,
        criticalCount: 0,
        signals: [],
      }),
    ).toEqual({
      severity: 'unknown',
      label: 'Unavailable',
      detail: 'no evaluable key signals',
    })
  })

  it('formats percent values', () => {
    expect(formatPercentValue(12.345)).toBe('12.3%')
    expect(formatPercentValue(12.345, 2)).toBe('12.35%')
    expect(formatPercentValue(-1)).toBe('-')
  })

  it('computes byte rates from adjacent samples', () => {
    expect(computeByteRate([100, 1124], [1000, 3000])).toBe(512)
    expect(computeByteRate([1124, 100], [1000, 3000])).toBe(0)
    expect(computeByteRate([100], [1000])).toBe(0)
  })

  it('formats byte rates', () => {
    expect(formatByteRate(0)).toBe('0 B/s')
    expect(formatByteRate(1536)).toBe('1.5 KB/s')
  })

  it('formats host uptime for operations views', () => {
    expect(formatDurationLong(59)).toBe('59s')
    expect(formatDurationLong(3600 + 120)).toBe('1h 2m')
    expect(formatDurationLong(2 * 86400 + 3 * 3600)).toBe('2d 3h')
  })
})
