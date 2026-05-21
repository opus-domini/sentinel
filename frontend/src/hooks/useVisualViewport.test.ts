// @vitest-environment jsdom
import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useVisualViewport } from './useVisualViewport'

type FakeVisualViewport = EventTarget & {
  width: number
  height: number
  offsetTop: number
  offsetLeft: number
}

function installVisualViewport(
  initial: Pick<
    FakeVisualViewport,
    'width' | 'height' | 'offsetTop' | 'offsetLeft'
  >,
): FakeVisualViewport {
  const target = new EventTarget() as FakeVisualViewport
  target.width = initial.width
  target.height = initial.height
  target.offsetTop = initial.offsetTop
  target.offsetLeft = initial.offsetLeft
  Object.defineProperty(window, 'visualViewport', {
    configurable: true,
    value: target,
  })
  return target
}

describe('useVisualViewport', () => {
  it('tracks keyboard visibility from viewport shrink and clears on restore', () => {
    const vv = installVisualViewport({
      width: 390,
      height: 800,
      offsetTop: 0,
      offsetLeft: 0,
    })

    renderHook(() => useVisualViewport())

    const root = document.documentElement
    expect(root.classList.contains('viewport-tracked')).toBe(true)
    expect(root.classList.contains('keyboard-visible')).toBe(false)

    vv.height = 620
    vv.dispatchEvent(new Event('resize'))

    expect(root.style.getPropertyValue('--keyboard-inset')).toBe('180px')
    expect(root.classList.contains('keyboard-visible')).toBe(true)

    vv.height = 800
    vv.dispatchEvent(new Event('resize'))

    expect(root.style.getPropertyValue('--keyboard-inset')).toBe('0px')
    expect(root.classList.contains('keyboard-visible')).toBe(false)
  })

  it('cleans viewport classes and css vars on unmount', () => {
    installVisualViewport({
      width: 390,
      height: 800,
      offsetTop: 0,
      offsetLeft: 0,
    })
    const { unmount } = renderHook(() => useVisualViewport())

    expect(
      document.documentElement.classList.contains('viewport-tracked'),
    ).toBe(true)
    unmount()

    const root = document.documentElement
    expect(root.classList.contains('viewport-tracked')).toBe(false)
    expect(root.classList.contains('keyboard-visible')).toBe(false)
    expect(root.style.getPropertyValue('--visual-viewport-height')).toBe('')
    expect(root.style.getPropertyValue('--keyboard-inset')).toBe('')
  })
})
