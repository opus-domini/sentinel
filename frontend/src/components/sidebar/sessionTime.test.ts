import { afterEach, describe, expect, it, vi } from 'vitest'

import { formatRelativeTime, formatTimestamp } from './sessionTime'

describe('formatRelativeTime', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns "-" for empty string', () => {
    expect(formatRelativeTime('')).toBe('-')
  })

  it('returns "-" for invalid date', () => {
    expect(formatRelativeTime('not-a-date')).toBe('-')
  })

  it('returns "now" for less than 60 seconds ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-01-15T12:00:30Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('now')
  })

  it('returns minutes for 1-59 minutes ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-01-15T12:05:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('5m')
  })

  it('returns hours for 1-23 hours ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-01-15T15:00:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('3h')
  })

  it('returns days for 1-6 days ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-01-18T12:00:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('3d')
  })

  it('returns weeks for 1-4 weeks ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-02-05T12:00:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('3w')
  })

  it('returns months for 1-11 months ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-06-15T12:00:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('5mo')
  })

  it('returns years for 1+ years ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2027-01-15T12:00:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('2y')
  })

  it('returns "1m" at exactly 60 seconds', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-01-15T12:01:00Z'))
    expect(formatRelativeTime('2025-01-15T12:00:00Z')).toBe('1m')
  })
})

describe('formatTimestamp', () => {
  it('returns "-" for empty string', () => {
    expect(formatTimestamp('')).toBe('-')
  })

  it('returns "-" for invalid date', () => {
    expect(formatTimestamp('not-a-date')).toBe('-')
  })

  it('returns a formatted date string for valid input', () => {
    const result = formatTimestamp('2025-01-15T12:30:00Z')
    // The exact output depends on locale, but it should contain
    // date and time portions and not be "-".
    expect(result).not.toBe('-')
    expect(result.length).toBeGreaterThan(5)
  })
})
