import { describe, expect, it } from 'vitest'

import {
  computeByteRate,
  fanSensorSeverity,
  formatByteRate,
  formatDurationLong,
  formatPercentValue,
  percentSeverity,
  presentMetricPosture,
  presentMetricSignal,
  powerSensorSeverity,
  pressureSeverity,
  sensorStatusLabel,
  temperatureSensorSeverity,
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

  it('maps every canonical posture signal to its stable owner tab', () => {
    expect(
      [
        'cpu',
        'memory',
        'rootDisk',
        'inodes',
        'swap',
        'cpuPressure',
        'memoryPressure',
        'ioPressure',
        'temperature',
        'fan',
        'power',
      ].map((signal) => presentMetricSignal(signal as Parameters<typeof presentMetricSignal>[0])),
    ).toEqual([
      { elementID: 'metric-signal-cpu', label: 'CPU', tab: 'saturation' },
      { elementID: 'metric-signal-memory', label: 'Memory', tab: 'saturation' },
      { elementID: 'metric-signal-rootDisk', label: 'Root disk', tab: 'saturation' },
      { elementID: 'metric-signal-inodes', label: 'Disk inodes', tab: 'saturation' },
      { elementID: 'metric-signal-swap', label: 'Swap', tab: 'saturation' },
      {
        elementID: 'metric-signal-cpuPressure',
        label: 'CPU pressure',
        tab: 'saturation',
      },
      {
        elementID: 'metric-signal-memoryPressure',
        label: 'Memory pressure',
        tab: 'saturation',
      },
      { elementID: 'metric-signal-ioPressure', label: 'IO pressure', tab: 'saturation' },
      { elementID: 'metric-signal-temperature', label: 'Temperature', tab: 'sensors' },
      { elementID: 'metric-signal-fan', label: 'Fans', tab: 'sensors' },
      { elementID: 'metric-signal-power', label: 'Power', tab: 'sensors' },
    ])
  })

  it('classifies hardware sensors only from reported thresholds and alarms', () => {
    expect(
      temperatureSensorSeverity({
        id: 'temp1',
        label: 'Package',
        source: 'coretemp',
        celsius: 81,
        maxCelsius: 80,
        criticalCelsius: 95,
      }),
    ).toBe('warn')
    expect(
      temperatureSensorSeverity({
        id: 'temp1',
        label: 'Package',
        source: 'coretemp',
        celsius: 60,
      }),
    ).toBe('unknown')
    expect(
      fanSensorSeverity({
        id: 'fan1',
        label: 'Chassis',
        source: 'nct6798',
        rpm: 0,
      }),
    ).toBe('unknown')
    expect(
      fanSensorSeverity({
        id: 'fan1',
        label: 'Chassis',
        source: 'nct6798',
        rpm: 0,
        alarm: true,
      }),
    ).toBe('critical')
    expect(
      powerSensorSeverity({
        id: 'power1',
        label: 'Package',
        source: 'zenpower',
        watts: 70,
        criticalWatts: 65,
      }),
    ).toBe('critical')
    expect(sensorStatusLabel('unknown')).toBe('measured')
  })

  it('presents the canonical backend posture without recomputing host thresholds', () => {
    expect(
      presentMetricPosture({
        state: 'pressure',
        severity: 'critical',
        warningCount: 1,
        criticalCount: 2,
        observedAt: '2026-07-27T12:00:30Z',
        signals: [
          {
            name: 'cpu',
            severity: 'warning',
            value: 85,
            since: '2026-07-27T12:00:10Z',
          },
          {
            name: 'memory',
            severity: 'critical',
            value: 95,
            since: '2026-07-27T12:00:20Z',
          },
          {
            name: 'ioPressure',
            severity: 'critical',
            value: 12,
            since: '2026-07-27T12:00:15Z',
          },
        ],
      }),
    ).toEqual({
      severity: 'critical',
      label: 'Critical',
      detail: '2 critical signals · 1 warning signal',
      observedAt: '2026-07-27T12:00:30Z',
      since: '2026-07-27T12:00:10Z',
    })
    expect(
      presentMetricPosture({
        state: 'unavailable',
        severity: 'unknown',
        warningCount: 0,
        criticalCount: 0,
        signals: [],
        observedAt: '2026-07-27T12:01:00Z',
      }),
    ).toEqual({
      severity: 'unknown',
      label: 'Unavailable',
      detail: 'no evaluable key signals',
      observedAt: '2026-07-27T12:01:00Z',
      since: null,
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
