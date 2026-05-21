import { describe, expect, it } from 'vitest'
import {
  buildTimelineQueryString,
  formatTimelineEventLocation,
  shouldRefreshTimelineFromEvent,
} from './tmuxTimeline'

describe('buildTimelineQueryString', () => {
  it('builds encoded query with active filters only', () => {
    const query = buildTimelineQueryString({
      session: 'dev',
      query: 'panic marker',
      severity: 'error',
      eventType: 'output.marker',
      limit: 120,
    })

    expect(query).toContain('session=dev')
    expect(query).toContain('q=panic+marker')
    expect(query).toContain('severity=error')
    expect(query).toContain('eventType=output.marker')
    expect(query).toContain('limit=120')
  })

  it('ignores all/empty values', () => {
    const query = buildTimelineQueryString({
      session: '  ',
      query: '',
      severity: 'all',
      eventType: 'all',
      limit: 0,
    })

    expect(query).toBe('')
  })
})

describe('shouldRefreshTimelineFromEvent', () => {
  it('refreshes for all scope when payload has sessions', () => {
    expect(shouldRefreshTimelineFromEvent(['dev'], 'all')).toBe(true)
  })

  it('refreshes only when tracked session is present', () => {
    expect(shouldRefreshTimelineFromEvent(['dev', 'ops'], 'dev')).toBe(true)
    expect(shouldRefreshTimelineFromEvent(['ops'], 'dev')).toBe(false)
  })

  it('returns false for invalid payload', () => {
    expect(shouldRefreshTimelineFromEvent(null, 'dev')).toBe(false)
    expect(shouldRefreshTimelineFromEvent([], '')).toBe(false)
  })
})

describe('formatTimelineEventLocation', () => {
  it('uses metadata names when available', () => {
    const result = formatTimelineEventLocation({
      id: 1,
      session: 'Sentinel',
      windowIndex: 0,
      paneId: '%1',
      eventType: 'output.marker',
      severity: 'warn',
      command: '',
      cwd: '',
      durationMs: 0,
      summary: '',
      details: '',
      marker: '',
      metadata: {
        windowName: 'node',
        paneTitle: 'optiplex',
      },
      createdAt: '2026-02-15T00:00:00Z',
    })

    expect(result.label).toBe('Sentinel > node > optiplex')
    expect(result.fallback).toBe('Sentinel:0:%1')
  })

  it('falls back to ids when names are missing', () => {
    const result = formatTimelineEventLocation({
      id: 2,
      session: 'Sentinel',
      windowIndex: 3,
      paneId: '%9',
      eventType: 'command.started',
      severity: 'info',
      command: '',
      cwd: '',
      durationMs: 0,
      summary: '',
      details: '',
      marker: '',
      metadata: null,
      createdAt: '2026-02-15T00:00:00Z',
    })

    expect(result.label).toBe('Sentinel > #3 > %9')
    expect(result.fallback).toBe('Sentinel:3:%9')
  })
})
