import { describe, expect, it } from 'vitest'

import { buildWSProtocols } from './wsAuth'

describe('buildWSProtocols', () => {
  it('returns only sentinel.v1 for empty token', () => {
    expect(buildWSProtocols('')).toEqual(['sentinel.v1'])
  })

  it('returns only sentinel.v1 for whitespace-only token', () => {
    expect(buildWSProtocols('   ')).toEqual(['sentinel.v1'])
  })

  it('appends base64url-encoded auth protocol for a token', () => {
    const protocols = buildWSProtocols('my-secret')
    expect(protocols).toHaveLength(2)
    expect(protocols[0]).toBe('sentinel.v1')
    expect(protocols[1]).toMatch(/^sentinel\.auth\./)
  })

  it('uses base64url encoding (no +, /, or =)', () => {
    // Use a token that would produce + or / in standard base64.
    const protocols = buildWSProtocols('test?token>>value')
    const encoded = protocols[1].replace('sentinel.auth.', '')
    expect(encoded).not.toContain('+')
    expect(encoded).not.toContain('/')
    expect(encoded).not.toContain('=')
  })

  it('round-trips through base64url decode', () => {
    const token = 'hello-world-123'
    const protocols = buildWSProtocols(token)
    const encoded = protocols[1].replace('sentinel.auth.', '')

    // Restore standard base64 and decode.
    const standard = encoded.replace(/-/g, '+').replace(/_/g, '/')
    const decoded = atob(standard)
    expect(decoded).toBe(token)
  })

  it('trims token before encoding', () => {
    const withSpaces = buildWSProtocols('  tok  ')
    const without = buildWSProtocols('tok')
    expect(withSpaces).toEqual(without)
  })
})
