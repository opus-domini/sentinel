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

    // Include the touch coordinates so xterm's mouse service can map the
    // event to terminal cell coordinates.  Without valid clientX/clientY,
    // getMouseReportCoords returns null and the mouse-protocol wheel
    // handler silently drops the event.
    const wheelEvent = new WheelEvent('wheel', {
      deltaY,
      deltaMode: WheelEvent.DOM_DELTA_PIXEL,
      bubbles: true,
      cancelable: true,
      clientX: touch.clientX,
      clientY: touch.clientY,
      screenX: touch.screenX,
      screenY: touch.screenY,
    })

    // Chrome (and some WebKit browsers) define the legacy `wheelDeltaY`
    // property on all WheelEvent instances—even synthetic ones created via
    // `new WheelEvent()`.  The value defaults to 0 because `wheelDeltaY`
    // is not part of the WheelEventInit dictionary.
    //
    // xterm 6.x's VS Code-derived StandardWheelEvent class checks
    // `typeof e.wheelDeltaY !== 'undefined'` and takes a legacy branch
    // that divides wheelDeltaY by 120.  With wheelDeltaY = 0, the
    // computed scroll delta is 0 and the SmoothScrollableElement does not
    // scroll.
    //
    // Override with the value Chrome would produce for a real event:
    // wheelDeltaY = -deltaY * 3  (pixel-mode convention).
    try {
      Object.defineProperty(wheelEvent, 'wheelDeltaY', {
        value: -deltaY * 3,
      })
      Object.defineProperty(wheelEvent, 'wheelDeltaX', { value: 0 })
    } catch {
      // If the property is non-configurable (unlikely), fall through.
    }

    dispatchTarget.dispatchEvent(wheelEvent)
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
