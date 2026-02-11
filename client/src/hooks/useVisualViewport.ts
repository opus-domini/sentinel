import { useEffect } from 'react'
import { isIOSDevice } from '@/lib/device'

const KEYBOARD_THRESHOLD = 100

export function useVisualViewport(): void {
  useEffect(() => {
    if (!isIOSDevice() || !window.visualViewport) return

    const vv = window.visualViewport
    const root = document.documentElement

    const update = () => {
      const keyboardInset = window.innerHeight - vv.height
      const keyboardVisible = keyboardInset > KEYBOARD_THRESHOLD

      root.style.setProperty('--keyboard-inset', `${keyboardInset}px`)
      root.style.setProperty('--viewport-offset-top', `${vv.offsetTop}px`)
      root.style.setProperty('--viewport-offset-left', `${vv.offsetLeft}px`)
      root.style.setProperty('--visual-viewport-height', `${vv.height}px`)
      root.style.setProperty('--visual-viewport-width', `${vv.width}px`)

      root.classList.toggle('keyboard-visible', keyboardVisible)
    }

    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)

    // RAF burst on input focus for smooth tracking
    const onFocusIn = (event: FocusEvent) => {
      const target = event.target as HTMLElement | null
      if (!target || !('tagName' in target)) return
      const tag = target.tagName.toLowerCase()
      if (tag !== 'input' && tag !== 'textarea' && tag !== 'select') return

      const start = performance.now()
      const tick = () => {
        update()
        if (performance.now() - start < 1500) {
          requestAnimationFrame(tick)
        }
      }
      requestAnimationFrame(tick)
    }

    document.addEventListener('focusin', onFocusIn)

    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
      document.removeEventListener('focusin', onFocusIn)
      root.style.removeProperty('--keyboard-inset')
      root.style.removeProperty('--viewport-offset-top')
      root.style.removeProperty('--viewport-offset-left')
      root.style.removeProperty('--visual-viewport-height')
      root.style.removeProperty('--visual-viewport-width')
      root.classList.remove('keyboard-visible')
    }
  }, [])
}
