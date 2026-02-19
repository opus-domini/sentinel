import { afterEach, describe, expect, it, vi } from 'vitest'

import { hapticFeedback, isIOSDevice, isSafari } from './device'

describe('isIOSDevice', () => {
  const originalNavigator = globalThis.navigator

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: originalNavigator,
      writable: true,
      configurable: true,
    })
  })

  function mockNavigator(userAgent: string, maxTouchPoints = 0): void {
    Object.defineProperty(globalThis, 'navigator', {
      value: { userAgent, maxTouchPoints },
      writable: true,
      configurable: true,
    })
  }

  it('returns true for iPhone', () => {
    mockNavigator('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)')
    expect(isIOSDevice()).toBe(true)
  })

  it('returns true for iPad', () => {
    mockNavigator('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)')
    expect(isIOSDevice()).toBe(true)
  })

  it('returns true for iPadOS (Macintosh UA with touch)', () => {
    mockNavigator(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15',
      5,
    )
    expect(isIOSDevice()).toBe(true)
  })

  it('returns false for Mac desktop', () => {
    mockNavigator(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15',
      0,
    )
    expect(isIOSDevice()).toBe(false)
  })

  it('returns false for Android', () => {
    mockNavigator(
      'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120',
    )
    expect(isIOSDevice()).toBe(false)
  })

  it('returns false when navigator is undefined', () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: undefined,
      writable: true,
      configurable: true,
    })
    expect(isIOSDevice()).toBe(false)
  })
})

describe('isSafari', () => {
  const originalNavigator = globalThis.navigator

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: originalNavigator,
      writable: true,
      configurable: true,
    })
  })

  function mockUA(userAgent: string): void {
    Object.defineProperty(globalThis, 'navigator', {
      value: { userAgent },
      writable: true,
      configurable: true,
    })
  }

  it('returns true for Safari', () => {
    mockUA(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
    )
    expect(isSafari()).toBe(true)
  })

  it('returns false for Chrome', () => {
    mockUA(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    )
    expect(isSafari()).toBe(false)
  })

  it('returns false for Edge', () => {
    mockUA(
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0',
    )
    expect(isSafari()).toBe(false)
  })

  it('returns false when navigator is undefined', () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: undefined,
      writable: true,
      configurable: true,
    })
    expect(isSafari()).toBe(false)
  })
})

describe('hapticFeedback', () => {
  const originalNavigator = globalThis.navigator

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: originalNavigator,
      writable: true,
      configurable: true,
    })
  })

  it('calls navigator.vibrate with default duration', () => {
    const vibrate = vi.fn()
    Object.defineProperty(globalThis, 'navigator', {
      value: { ...originalNavigator, vibrate },
      writable: true,
      configurable: true,
    })
    hapticFeedback()
    expect(vibrate).toHaveBeenCalledWith(10)
  })

  it('calls navigator.vibrate with custom duration', () => {
    const vibrate = vi.fn()
    Object.defineProperty(globalThis, 'navigator', {
      value: { ...originalNavigator, vibrate },
      writable: true,
      configurable: true,
    })
    hapticFeedback(50)
    expect(vibrate).toHaveBeenCalledWith(50)
  })

  it('does not throw when vibrate is not available', () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: { userAgent: 'test' },
      writable: true,
      configurable: true,
    })
    expect(() => hapticFeedback()).not.toThrow()
  })
})
