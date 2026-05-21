import { describe, expect, it } from 'vitest'

import {
  connectionBadgeClass,
  connectionDotClass,
  connectionLabel,
} from './connection'
import type { ConnectionState } from '../types'

const states: Array<ConnectionState> = [
  'connected',
  'connecting',
  'disconnected',
  'error',
]

describe('connectionBadgeClass', () => {
  it.each(states)('returns a non-empty class string for "%s"', (state) => {
    const result = connectionBadgeClass(state)
    expect(result).toBeTruthy()
    expect(typeof result).toBe('string')
  })

  it('returns correct class for connected', () => {
    expect(connectionBadgeClass('connected')).toContain('ok')
  })

  it('returns correct class for connecting', () => {
    expect(connectionBadgeClass('connecting')).toContain('warning')
  })

  it('returns correct class for error', () => {
    expect(connectionBadgeClass('error')).toContain('destructive')
  })

  it('returns correct class for disconnected', () => {
    expect(connectionBadgeClass('disconnected')).toContain('muted-foreground')
  })
})

describe('connectionDotClass', () => {
  it.each(states)('returns a non-empty class string for "%s"', (state) => {
    const result = connectionDotClass(state)
    expect(result).toBeTruthy()
    expect(typeof result).toBe('string')
  })

  it('returns correct class for connected', () => {
    expect(connectionDotClass('connected')).toContain('ok')
  })

  it('returns correct class for connecting', () => {
    expect(connectionDotClass('connecting')).toContain('warning')
  })

  it('returns correct class for error', () => {
    expect(connectionDotClass('error')).toContain('destructive')
  })

  it('returns correct class for disconnected', () => {
    expect(connectionDotClass('disconnected')).toContain('muted-foreground')
  })
})

describe('connectionLabel', () => {
  it.each([
    ['connected', 'Connected'],
    ['connecting', 'Connecting'],
    ['disconnected', 'Disconnected'],
    ['error', 'Error'],
  ] as Array<[ConnectionState, string]>)(
    'returns "%s" → "%s"',
    (state, expected) => {
      expect(connectionLabel(state)).toBe(expected)
    },
  )
})
