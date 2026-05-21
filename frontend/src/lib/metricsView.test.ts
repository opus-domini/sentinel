import { describe, expect, it } from 'vitest'

import {
  computeByteRate,
  formatByteRate,
  formatDurationLong,
  formatPercentValue,
  percentSeverity,
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
