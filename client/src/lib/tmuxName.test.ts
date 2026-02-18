import { describe, expect, it } from 'vitest'

import { slugifyTmuxName } from './tmuxName'

describe('slugifyTmuxName', () => {
  it('passes through valid names unchanged', () => {
    expect(slugifyTmuxName('my-session')).toBe('my-session')
    expect(slugifyTmuxName('dev.api')).toBe('dev.api')
    expect(slugifyTmuxName('build_v2')).toBe('build_v2')
  })

  it('replaces spaces with hyphens', () => {
    expect(slugifyTmuxName('my session')).toBe('my-session')
    expect(slugifyTmuxName('a  b  c')).toBe('a-b-c')
  })

  it('strips invalid characters', () => {
    expect(slugifyTmuxName('hello@world!')).toBe('helloworld')
    expect(slugifyTmuxName('test#$%')).toBe('test')
    expect(slugifyTmuxName('a/b\\c')).toBe('abc')
  })

  it('truncates to 64 characters', () => {
    const long = 'a'.repeat(100)
    expect(slugifyTmuxName(long)).toBe('a'.repeat(64))
  })

  it('returns empty string for empty input', () => {
    expect(slugifyTmuxName('')).toBe('')
  })

  it('returns empty string for all-invalid characters', () => {
    expect(slugifyTmuxName('!@#$%^&*()')).toBe('')
  })

  it('preserves dots, hyphens, and underscores', () => {
    expect(slugifyTmuxName('a.b-c_d')).toBe('a.b-c_d')
  })

  it('handles mixed valid and invalid with spaces', () => {
    expect(slugifyTmuxName('My Session (2)')).toBe('My-Session-2')
  })
})
