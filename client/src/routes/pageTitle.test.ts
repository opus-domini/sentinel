import { describe, expect, it } from 'vitest'

import { formatPageTitle } from './pageTitle'

describe('formatPageTitle', () => {
  it('returns the default title when hostname is missing', () => {
    expect(formatPageTitle()).toBe('Sentinel')
    expect(formatPageTitle('')).toBe('Sentinel')
    expect(formatPageTitle(null)).toBe('Sentinel')
  })

  it('puts the hostname before the app name', () => {
    expect(formatPageTitle('host-01')).toBe('host-01 - Sentinel')
  })
})
