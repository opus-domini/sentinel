// @vitest-environment jsdom
import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useEdgeSwipe } from './useEdgeSwipe'

function touch(x: number, y: number): Touch {
  return { clientX: x, clientY: y } as Touch
}

function fireTouchStart(x: number, y: number): void {
  document.dispatchEvent(
    new TouchEvent('touchstart', { touches: [touch(x, y)] }),
  )
}

function fireTouchMove(x: number, y: number): void {
  document.dispatchEvent(
    new TouchEvent('touchmove', { touches: [touch(x, y)] }),
  )
}

function fireTouchEnd(): void {
  document.dispatchEvent(new TouchEvent('touchend'))
}

describe('useEdgeSwipe', () => {
  it('calls onSwipeOpen on a right swipe from left edge', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(10, 100)
    fireTouchMove(70, 105)

    expect(onSwipeOpen).toHaveBeenCalledTimes(1)
  })

  it('does not trigger when disabled', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: false, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(10, 100)
    fireTouchMove(70, 105)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('does not trigger when sidebar is already open', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() => useEdgeSwipe({ enabled: true, isOpen: true, onSwipeOpen }))

    fireTouchStart(10, 100)
    fireTouchMove(70, 105)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('does not trigger when touch starts away from edge', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(100, 100)
    fireTouchMove(200, 105)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('does not trigger for short swipe', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(10, 100)
    fireTouchMove(30, 100)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('does not trigger for vertical swipe', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(10, 100)
    fireTouchMove(70, 200)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('resets tracking on touchend', () => {
    const onSwipeOpen = vi.fn()

    renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen }),
    )

    fireTouchStart(10, 100)
    fireTouchEnd()
    fireTouchMove(70, 105)

    expect(onSwipeOpen).not.toHaveBeenCalled()
  })

  it('cleans up listeners on unmount', () => {
    const removeSpy = vi.spyOn(document, 'removeEventListener')

    const { unmount } = renderHook(() =>
      useEdgeSwipe({ enabled: true, isOpen: false, onSwipeOpen: vi.fn() }),
    )

    unmount()

    const removedEvents = removeSpy.mock.calls.map((c) => c[0])
    expect(removedEvents).toContain('touchstart')
    expect(removedEvents).toContain('touchmove')
    expect(removedEvents).toContain('touchend')

    removeSpy.mockRestore()
  })
})
