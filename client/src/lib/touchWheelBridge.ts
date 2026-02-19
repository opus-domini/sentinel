type TouchWheelBridgeOptions = {
  host: HTMLElement
  dispatchTarget: HTMLElement
}

const BOTTOM_GESTURE_GUTTER = 56

const noopDispose = {
  dispose: () => undefined,
}

function isNearBottomGestureArea(touch: Touch): boolean {
  const vv = window.visualViewport
  const viewportBottom = vv ? vv.height : window.innerHeight
  return touch.clientY >= viewportBottom - BOTTOM_GESTURE_GUTTER
}

function isBlockedTouchTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) {
    return false
  }
  return target.closest('[data-sentinel-touch-lock]') !== null
}

export function attachTouchWheelBridge({
  host,
  dispatchTarget,
}: TouchWheelBridgeOptions): { dispose: () => void } {
  if (!host.isConnected || !dispatchTarget.isConnected) {
    return noopDispose
  }

  let activeTouchID: number | null = null
  let lastY = 0

  const reset = () => {
    activeTouchID = null
    lastY = 0
  }

  const findActiveTouch = (touches: TouchList): Touch | null => {
    if (activeTouchID === null) {
      return null
    }
    const list = touches as unknown as ArrayLike<Touch>
    for (let index = 0; index < touches.length; index += 1) {
      const touch = list[index]
      if (touch.identifier === activeTouchID) {
        return touch
      }
    }
    return null
  }

  const onTouchStart = (event: TouchEvent) => {
    if (event.touches.length !== 1) {
      reset()
      return
    }

    if (isBlockedTouchTarget(event.target)) {
      reset()
      return
    }

    const list = event.touches as unknown as ArrayLike<Touch>
    const touch = list[0]
    if (isNearBottomGestureArea(touch)) {
      reset()
      return
    }
    if (document.documentElement.classList.contains('keyboard-visible')) {
      event.preventDefault()
    }
    activeTouchID = touch.identifier
    lastY = touch.clientY
  }

  const onTouchMove = (event: TouchEvent) => {
    const touch = findActiveTouch(event.touches)
    if (!touch) {
      return
    }
    const deltaY = lastY - touch.clientY
    lastY = touch.clientY
    if (Math.abs(deltaY) < 0.5) {
      return
    }
    event.preventDefault()
    dispatchTarget.dispatchEvent(
      new WheelEvent('wheel', {
        deltaY,
        deltaMode: WheelEvent.DOM_DELTA_PIXEL,
        bubbles: true,
        cancelable: true,
      }),
    )
  }

  const onTouchEnd = (event: TouchEvent) => {
    if (activeTouchID === null) {
      return
    }
    if (findActiveTouch(event.touches) === null) {
      reset()
    }
  }

  const onTouchCancel = () => {
    reset()
  }

  host.addEventListener('touchstart', onTouchStart, { passive: false })
  host.addEventListener('touchmove', onTouchMove, { passive: false })
  host.addEventListener('touchend', onTouchEnd, { passive: true })
  host.addEventListener('touchcancel', onTouchCancel, { passive: true })

  return {
    dispose: () => {
      host.removeEventListener('touchstart', onTouchStart)
      host.removeEventListener('touchmove', onTouchMove)
      host.removeEventListener('touchend', onTouchEnd)
      host.removeEventListener('touchcancel', onTouchCancel)
    },
  }
}
