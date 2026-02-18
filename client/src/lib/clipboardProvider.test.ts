import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createWebClipboardProvider } from './clipboardProvider'

describe('createWebClipboardProvider', () => {
  const originalNavigator = globalThis.navigator
  let mockWriteText: ReturnType<typeof vi.fn>
  let mockReadText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    mockWriteText = vi.fn().mockResolvedValue(undefined)
    mockReadText = vi.fn().mockResolvedValue('clipboard content')
    Object.defineProperty(globalThis, 'navigator', {
      value: {
        ...originalNavigator,
        clipboard: {
          writeText: mockWriteText,
          readText: mockReadText,
        },
      },
      writable: true,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: originalNavigator,
      writable: true,
      configurable: true,
    })
  })

  describe('writeText', () => {
    it('writes to clipboard for system selection (c)', async () => {
      const provider = createWebClipboardProvider()
      await provider.writeText('c' as never, 'hello')
      expect(mockWriteText).toHaveBeenCalledWith('hello')
    })

    it('writes to clipboard for primary selection (p)', async () => {
      const provider = createWebClipboardProvider()
      await provider.writeText('p' as never, 'hello')
      expect(mockWriteText).toHaveBeenCalledWith('hello')
    })

    it('writes to clipboard for empty selection (tmux default)', async () => {
      const provider = createWebClipboardProvider()
      // tmux sends OSC 52 with an empty selection string when
      // set-clipboard is on.  The default BrowserClipboardProvider
      // silently ignores this because '' !== 'c'.
      await provider.writeText('' as never, 'hello from tmux')
      expect(mockWriteText).toHaveBeenCalledWith('hello from tmux')
    })

    it('writes to clipboard for numeric selection buffers', async () => {
      const provider = createWebClipboardProvider()
      await provider.writeText('0' as never, 'buffer zero')
      expect(mockWriteText).toHaveBeenCalledWith('buffer zero')
    })

    it('swallows clipboard write errors gracefully', async () => {
      mockWriteText.mockRejectedValue(new DOMException('not allowed'))
      const provider = createWebClipboardProvider()
      // Should not throw even when navigator.clipboard.writeText rejects.
      await expect(
        provider.writeText('c' as never, 'hello'),
      ).resolves.toBeUndefined()
    })
  })

  describe('readText', () => {
    it('reads from clipboard', async () => {
      const provider = createWebClipboardProvider()
      const result = await provider.readText('c' as never)
      expect(result).toBe('clipboard content')
    })

    it('returns empty string on read error', async () => {
      mockReadText.mockRejectedValue(new DOMException('not focused'))
      const provider = createWebClipboardProvider()
      const result = await provider.readText('c' as never)
      expect(result).toBe('')
    })

    it('returns empty string when clipboard API is missing', async () => {
      Object.defineProperty(globalThis, 'navigator', {
        value: { ...originalNavigator, clipboard: undefined },
        writable: true,
        configurable: true,
      })
      const provider = createWebClipboardProvider()
      const result = await provider.readText('c' as never)
      expect(result).toBe('')
    })
  })
})
