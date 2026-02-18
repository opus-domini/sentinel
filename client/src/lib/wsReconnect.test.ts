import { describe, expect, it } from 'vitest'

import { createReconnect } from './wsReconnect'

describe('createReconnect', () => {
  it('starts with 1200ms delay', () => {
    const rc = createReconnect()
    expect(rc.delay).toBe(1_200)
  })

  it('returns current delay and advances on next()', () => {
    const rc = createReconnect()
    expect(rc.next()).toBe(1_200)
    expect(rc.delay).toBe(2_040) // 1200 * 1.7
  })

  it('grows exponentially with factor 1.7', () => {
    const rc = createReconnect()
    const d1 = rc.next() // 1200
    const d2 = rc.next() // 2040
    const d3 = rc.next() // 3468

    expect(d1).toBe(1_200)
    expect(d2).toBe(2_040)
    expect(d3).toBe(3_468)
  })

  it('caps at 30000ms', () => {
    const rc = createReconnect()
    // Advance until we hit the cap.
    for (let i = 0; i < 20; i++) {
      rc.next()
    }
    expect(rc.delay).toBeLessThanOrEqual(30_000)
    expect(rc.next()).toBeLessThanOrEqual(30_000)
  })

  it('resets delay back to minimum', () => {
    const rc = createReconnect()
    rc.next()
    rc.next()
    expect(rc.delay).toBeGreaterThan(1_200)

    rc.reset()
    expect(rc.delay).toBe(1_200)
  })
})
