// @vitest-environment jsdom
import { act, fireEvent, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useShellLayout } from './useShellLayout'

function renderLayout(onResizeEnd = vi.fn()) {
  return renderHook(() =>
    useShellLayout({
      storageKey: 'sentinel_shell_test',
      defaultSidebarWidth: 340,
      minSidebarWidth: 240,
      maxSidebarWidth: 440,
      onResizeEnd,
    }),
  )
}

describe('useShellLayout', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    )
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(
      (callback: FrameRequestCallback) => {
        callback(0)
        return 1
      },
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    window.localStorage.clear()
  })

  it('clamps keyboard-driven sidebar resizing to configured bounds', () => {
    const onResizeEnd = vi.fn()
    const { result } = renderLayout(onResizeEnd)

    act(() => {
      result.current.resizeSidebarBy(500)
    })
    expect(result.current.sidebarWidth).toBe(440)

    act(() => {
      result.current.resizeSidebarTo(1)
    })
    expect(result.current.sidebarWidth).toBe(240)
    expect(onResizeEnd).toHaveBeenCalledTimes(2)
  })

  it('toggles the sidebar collapsed state with Ctrl+\\', () => {
    const { result } = renderLayout()

    expect(result.current.sidebarCollapsed).toBe(false)

    act(() => {
      expect(fireEvent.keyDown(document, { key: '\\', ctrlKey: true })).toBe(
        false,
      )
    })
    expect(result.current.sidebarCollapsed).toBe(true)

    const input = document.createElement('input')
    document.body.append(input)

    act(() => {
      expect(fireEvent.keyDown(input, { key: '\\', ctrlKey: true })).toBe(true)
    })
    expect(result.current.sidebarCollapsed).toBe(true)

    const terminal = document.createElement('div')
    terminal.className = 'xterm'
    document.body.append(terminal)

    act(() => {
      expect(fireEvent.keyDown(terminal, { key: '\\', ctrlKey: true })).toBe(
        false,
      )
    })
    expect(result.current.sidebarCollapsed).toBe(false)

    input.remove()
    terminal.remove()
  })
})
