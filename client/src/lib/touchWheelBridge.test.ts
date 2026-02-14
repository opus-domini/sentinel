// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'

import { attachTouchWheelBridge } from './touchWheelBridge'

function touch(identifier: number, x: number, y: number): Touch {
  return { identifier, clientX: x, clientY: y } as Touch
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

    host.dispatchEvent(
      new TouchEvent('touchstart', { touches: [touch(7, 10, 100)] }),
    )
    host.dispatchEvent(
      new TouchEvent('touchmove', { touches: [touch(7, 10, 80)] }),
    )

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

    host.dispatchEvent(
      new TouchEvent('touchstart', {
        touches: [touch(1, 10, 100), touch(2, 12, 98)],
      }),
    )
    host.dispatchEvent(
      new TouchEvent('touchmove', { touches: [touch(1, 10, 80)] }),
    )

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

    host.dispatchEvent(
      new TouchEvent('touchstart', { touches: [touch(9, 10, 90)] }),
    )
    host.dispatchEvent(
      new TouchEvent('touchmove', { touches: [touch(9, 10, 60)] }),
    )

    expect(wheelSpy).not.toHaveBeenCalled()
    document.body.removeChild(host)
  })
})
