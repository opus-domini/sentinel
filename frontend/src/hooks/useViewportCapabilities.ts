import { useEffect, useState } from 'react'

const COMPACT_LAYOUT_QUERY = '(max-width: 767px), (max-width: 1024px) and (max-height: 500px)'
const COARSE_POINTER_QUERY = '(pointer: coarse)'
const ANY_COARSE_POINTER_QUERY = '(any-pointer: coarse)'

export type ViewportCapabilities = {
  compactLayout: boolean
  touchCapable: boolean
  touchOptimized: boolean
}

function readCapabilities(): ViewportCapabilities {
  const compactLayout = window.matchMedia(COMPACT_LAYOUT_QUERY).matches
  const touchCapable =
    navigator.maxTouchPoints > 0 ||
    window.matchMedia(COARSE_POINTER_QUERY).matches ||
    window.matchMedia(ANY_COARSE_POINTER_QUERY).matches

  return {
    compactLayout,
    touchCapable,
    touchOptimized: compactLayout || touchCapable,
  }
}

export function useViewportCapabilities(): ViewportCapabilities {
  const [capabilities, setCapabilities] = useState(readCapabilities)

  useEffect(() => {
    const mediaQueries = [
      window.matchMedia(COMPACT_LAYOUT_QUERY),
      window.matchMedia(COARSE_POINTER_QUERY),
      window.matchMedia(ANY_COARSE_POINTER_QUERY),
    ]
    const updateCapabilities = () => setCapabilities(readCapabilities())

    for (const mediaQuery of mediaQueries) {
      mediaQuery.addEventListener('change', updateCapabilities)
    }

    updateCapabilities()

    return () => {
      for (const mediaQuery of mediaQueries) {
        mediaQuery.removeEventListener('change', updateCapabilities)
      }
    }
  }, [])

  return capabilities
}
