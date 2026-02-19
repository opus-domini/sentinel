import { useEffect } from 'react'

type UseEdgeSwipeOptions = {
  enabled: boolean
  isOpen: boolean
  onSwipeOpen: () => void
}

const EDGE_THRESHOLD = 30
const SWIPE_DISTANCE = 50
const SWIPE_RATIO = 1.5

export function useEdgeSwipe({
  enabled,
  isOpen,
  onSwipeOpen,
}: UseEdgeSwipeOptions): void {
  useEffect(() => {
    if (!enabled || isOpen) return

    let startX = 0
    let startY = 0
    let tracking = false

    const onTouchStart = (event: TouchEvent) => {
      const touch = event.touches[0]
      if (touch.clientX <= EDGE_THRESHOLD) {
        startX = touch.clientX
        startY = touch.clientY
        tracking = true
      }
    }

    const onTouchMove = (event: TouchEvent) => {
      if (!tracking) return
      const touch = event.touches[0]
      const dx = touch.clientX - startX
      const dy = Math.abs(touch.clientY - startY)

      if (dx >= SWIPE_DISTANCE && dx > dy * SWIPE_RATIO) {
        tracking = false
        onSwipeOpen()
      }
    }

    const onTouchEnd = () => {
      tracking = false
    }

    document.addEventListener('touchstart', onTouchStart, { passive: true })
    document.addEventListener('touchmove', onTouchMove, { passive: true })
    document.addEventListener('touchend', onTouchEnd, { passive: true })

    return () => {
      document.removeEventListener('touchstart', onTouchStart)
      document.removeEventListener('touchmove', onTouchMove)
      document.removeEventListener('touchend', onTouchEnd)
    }
  }, [enabled, isOpen, onSwipeOpen])
}
