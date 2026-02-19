// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'

import { attachTouchWheelBridge } from './touchWheelBridge'

function touch(identifier: number, x: number, y: number): Touch {
  return { identifier, clientX: x, clientY: y } as Touch
}

function touchEvent(type: string, touches: ReadonlyArray<Touch>): TouchEvent {
  return new TouchEvent(type, { touches, bubbles: true, cancelable: true })
}

function withVisualViewport(height: number): () => void {
  const previous = window.visualViewport
  const viewport = new EventTarget() as EventTarget & {
    width: number
    height: number
    offsetTop: number
    offsetLeft: number
  }
  viewport.width = window.innerWidth
  viewport.height = height
  viewport.offsetTop = 0
  viewport.offsetLeft = 0
  Object.defineProperty(window, 'visualViewport', {
    configurable: true,
    value: viewport,
  })
  return () => {
    Object.defineProperty(window, 'visualViewport', {
      configurable: true,
      value: previous,
    })
  }
}

describe('attachTouchWheelBridge', () => {
  it('dispatches wheel events from single-finger vertical touch movement', () => {
    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })

    target.dispatchEvent(touchEvent('touchstart', [touch(7, 10, 100)]))
    target.dispatchEvent(touchEvent('touchmove', [touch(7, 10, 80)]))

    expect(wheelSpy).toHaveBeenCalledTimes(1)
    const event = wheelSpy.mock.calls[0][0] as WheelEvent
    expect(event.deltaY).toBe(20)

    bridge.dispose()
    document.body.removeChild(host)
  })

  it('does not dispatch when gesture has multiple touches', () => {
    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })

    target.dispatchEvent(
      touchEvent('touchstart', [touch(1, 10, 100), touch(2, 12, 98)]),
    )
    target.dispatchEvent(touchEvent('touchmove', [touch(1, 10, 80)]))

    expect(wheelSpy).not.toHaveBeenCalled()

    bridge.dispose()
    document.body.removeChild(host)
  })

  it('cleans up listeners on dispose', () => {
    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })
    bridge.dispose()

    target.dispatchEvent(touchEvent('touchstart', [touch(9, 10, 90)]))
    target.dispatchEvent(touchEvent('touchmove', [touch(9, 10, 60)]))

    expect(wheelSpy).not.toHaveBeenCalled()
    document.body.removeChild(host)
  })

  it('ignores touches that start near bottom gesture area', () => {
    const restoreViewport = withVisualViewport(600)
    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })

    target.dispatchEvent(touchEvent('touchstart', [touch(5, 10, 590)]))
    target.dispatchEvent(touchEvent('touchmove', [touch(5, 10, 540)]))

    expect(wheelSpy).not.toHaveBeenCalled()

    bridge.dispose()
    document.body.removeChild(host)
    restoreViewport()
  })

  it('keeps bottom guard when visualViewport has non-zero offsetTop', () => {
    const previous = window.visualViewport
    const viewport = new EventTarget() as EventTarget & {
      width: number
      height: number
      offsetTop: number
      offsetLeft: number
    }
    viewport.width = window.innerWidth
    viewport.height = 600
    viewport.offsetTop = 120
    viewport.offsetLeft = 0
    Object.defineProperty(window, 'visualViewport', {
      configurable: true,
      value: viewport,
    })

    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })
    target.dispatchEvent(touchEvent('touchstart', [touch(11, 10, 590)]))
    target.dispatchEvent(touchEvent('touchmove', [touch(11, 10, 550)]))

    expect(wheelSpy).not.toHaveBeenCalled()

    bridge.dispose()
    document.body.removeChild(host)
    Object.defineProperty(window, 'visualViewport', {
      configurable: true,
      value: previous,
    })
  })

  it('ignores touches from locked zones and from outside host tree', () => {
    const host = document.createElement('div')
    const target = document.createElement('div')
    const outside = document.createElement('button')
    const locked = document.createElement('div')
    locked.setAttribute('data-sentinel-touch-lock', '')

    host.appendChild(target)
    target.appendChild(locked)
    document.body.appendChild(outside)
    document.body.appendChild(host)

    const wheelSpy = vi.fn()
    target.addEventListener('wheel', wheelSpy as EventListener)

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })

    outside.dispatchEvent(touchEvent('touchstart', [touch(1, 10, 100)]))
    outside.dispatchEvent(touchEvent('touchmove', [touch(1, 10, 70)]))
    locked.dispatchEvent(touchEvent('touchstart', [touch(2, 10, 100)]))
    locked.dispatchEvent(touchEvent('touchmove', [touch(2, 10, 70)]))

    expect(wheelSpy).not.toHaveBeenCalled()

    bridge.dispose()
    document.body.removeChild(outside)
    document.body.removeChild(host)
  })

  it('prevents default on touchstart when keyboard is visible', () => {
    const host = document.createElement('div')
    const target = document.createElement('div')
    host.appendChild(target)
    document.body.appendChild(host)
    document.documentElement.classList.add('keyboard-visible')

    const bridge = attachTouchWheelBridge({ host, dispatchTarget: target })

    const start = touchEvent('touchstart', [touch(3, 10, 120)])
    target.dispatchEvent(start)
    expect(start.defaultPrevented).toBe(true)

    bridge.dispose()
    document.documentElement.classList.remove('keyboard-visible')
    document.body.removeChild(host)
  })
})
