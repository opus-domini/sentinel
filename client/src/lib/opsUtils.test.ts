import { describe, expect, it } from 'vitest'

import {
  browsedServiceDot,
  formatBytes,
  formatUptime,
  toErrorMessage,
} from './opsUtils'

describe('formatUptime', () => {
  it('returns seconds for < 60s', () => {
    expect(formatUptime(45)).toBe('45s')
  })

  it('returns 0s for zero', () => {
    expect(formatUptime(0)).toBe('0s')
  })

  it('returns 0s for negative values', () => {
    expect(formatUptime(-100)).toBe('0s')
  })

  it('returns minutes for 60-3599s', () => {
    expect(formatUptime(120)).toBe('2m')
    expect(formatUptime(300)).toBe('5m')
  })

  it('returns hours and minutes for >= 3600s', () => {
    expect(formatUptime(3600)).toBe('1h 0m')
    expect(formatUptime(3661)).toBe('1h 1m')
    expect(formatUptime(7200)).toBe('2h 0m')
    expect(formatUptime(7380)).toBe('2h 3m')
  })

  it('truncates fractional seconds', () => {
    expect(formatUptime(59.9)).toBe('59s')
    expect(formatUptime(3600.7)).toBe('1h 0m')
  })
})

describe('formatBytes', () => {
  it('returns 0 B for zero', () => {
    expect(formatBytes(0)).toBe('0 B')
  })

  it('returns 0 B for negative values', () => {
    expect(formatBytes(-1024)).toBe('0 B')
  })

  it('returns bytes for small values', () => {
    expect(formatBytes(512)).toBe('512 B')
  })

  it('returns KB for kilobyte range', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('returns MB for megabyte range', () => {
    expect(formatBytes(1048576)).toBe('1.0 MB')
  })

  it('returns GB for gigabyte range', () => {
    expect(formatBytes(1073741824)).toBe('1.0 GB')
  })

  it('returns TB for terabyte range', () => {
    expect(formatBytes(1099511627776)).toBe('1.0 TB')
  })

  it('caps at TB for very large values', () => {
    const petabyte = 1099511627776 * 1024
    expect(formatBytes(petabyte)).toBe('1024 TB')
  })
})

describe('toErrorMessage', () => {
  it('returns Error.message for Error instances', () => {
    expect(toErrorMessage(new Error('oops'), 'fallback')).toBe('oops')
  })

  it('returns fallback for non-Error values', () => {
    expect(toErrorMessage('string error', 'fallback')).toBe('fallback')
    expect(toErrorMessage(42, 'fallback')).toBe('fallback')
    expect(toErrorMessage(null, 'fallback')).toBe('fallback')
    expect(toErrorMessage(undefined, 'fallback')).toBe('fallback')
  })

  it('returns fallback for Error with empty message', () => {
    expect(toErrorMessage(new Error(''), 'fallback')).toBe('fallback')
    expect(toErrorMessage(new Error('  '), 'fallback')).toBe('fallback')
  })
})

describe('browsedServiceDot', () => {
  it('returns green for active', () => {
    expect(browsedServiceDot('active')).toBe('bg-emerald-500')
  })

  it('returns green for running', () => {
    expect(browsedServiceDot('running')).toBe('bg-emerald-500')
  })

  it('is case-insensitive', () => {
    expect(browsedServiceDot('Active')).toBe('bg-emerald-500')
    expect(browsedServiceDot('RUNNING')).toBe('bg-emerald-500')
    expect(browsedServiceDot('Failed')).toBe('bg-red-500')
  })

  it('trims whitespace', () => {
    expect(browsedServiceDot('  active  ')).toBe('bg-emerald-500')
  })

  it('returns red for failed', () => {
    expect(browsedServiceDot('failed')).toBe('bg-red-500')
  })

  it('returns muted for unknown states', () => {
    expect(browsedServiceDot('inactive')).toBe('bg-muted-foreground/50')
    expect(browsedServiceDot('stopped')).toBe('bg-muted-foreground/50')
    expect(browsedServiceDot('')).toBe('bg-muted-foreground/50')
  })
})
