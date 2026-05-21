import { describe, expect, it } from 'vitest'

import {
  effectiveAttachedClients,
  isSessionAttached,
  isSessionAttachedWithLocalTab,
} from './sessionAttachment'

describe('isSessionAttachedWithLocalTab', () => {
  it('returns true when backend has attached clients', () => {
    expect(isSessionAttachedWithLocalTab({ attached: 2 }, false)).toBe(true)
  })

  it('returns true when local tab is open', () => {
    expect(isSessionAttachedWithLocalTab({ attached: 0 }, true)).toBe(true)
  })

  it('returns false when detached in backend and local tab closed', () => {
    expect(isSessionAttachedWithLocalTab({ attached: 0 }, false)).toBe(false)
  })
})

describe('isSessionAttached', () => {
  it('returns true when backend reports attached clients', () => {
    expect(
      isSessionAttached({ name: 'dev', attached: 2 }, new Set<string>()),
    ).toBe(true)
  })

  it('returns true when session tab is open locally (optimistic attach)', () => {
    expect(
      isSessionAttached({ name: 'dev', attached: 0 }, new Set(['dev'])),
    ).toBe(true)
  })

  it('returns false when detached in backend and locally', () => {
    expect(
      isSessionAttached({ name: 'dev', attached: 0 }, new Set(['other'])),
    ).toBe(false)
  })
})

describe('effectiveAttachedClients', () => {
  it('keeps backend count when it is already greater than optimistic value', () => {
    expect(effectiveAttachedClients(3, true)).toBe(3)
  })

  it('uses 1 as optimistic fallback while local tab is open', () => {
    expect(effectiveAttachedClients(0, true)).toBe(1)
  })

  it('returns backend value when local tab is closed', () => {
    expect(effectiveAttachedClients(0, false)).toBe(0)
  })
})
