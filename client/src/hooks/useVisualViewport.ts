import { useEffect } from 'react'

const KEYBOARD_THRESHOLD = 100

export function useVisualViewport(): void {
  useEffect(() => {
    if (!window.visualViewport) return

    const vv = window.visualViewport
    const root = document.documentElement
    root.classList.add('viewport-tracked')
    let baselineHeight = vv.height

    const update = () => {
      if (vv.height > baselineHeight) {
        baselineHeight = vv.height
      }
      const keyboardInset = Math.max(0, baselineHeight - vv.height)
      const keyboardVisible = keyboardInset > KEYBOARD_THRESHOLD

      root.style.setProperty(
        '--keyboard-inset',
        `${keyboardVisible ? keyboardInset : 0}px`,
      )
      const offsetTop = keyboardVisible ? 0 : vv.offsetTop
      const offsetLeft = keyboardVisible ? 0 : vv.offsetLeft
      root.style.setProperty('--viewport-offset-top', `${offsetTop}px`)
      root.style.setProperty('--viewport-offset-left', `${offsetLeft}px`)
      root.style.setProperty('--visual-viewport-height', `${vv.height}px`)
      root.style.setProperty('--visual-viewport-width', `${vv.width}px`)

      root.classList.toggle('keyboard-visible', keyboardVisible)
    }

    const onOrientationChange = () => {
      baselineHeight = vv.height
      update()
    }

    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    window.addEventListener('orientationchange', onOrientationChange)

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
    update()

    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
      window.removeEventListener('orientationchange', onOrientationChange)
      document.removeEventListener('focusin', onFocusIn)
      root.style.removeProperty('--keyboard-inset')
      root.style.removeProperty('--viewport-offset-top')
      root.style.removeProperty('--viewport-offset-left')
      root.style.removeProperty('--visual-viewport-height')
      root.style.removeProperty('--visual-viewport-width')
      root.classList.remove('keyboard-visible')
      root.classList.remove('viewport-tracked')
    }
  }, [])
}
