import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'

type UseShellLayoutOptions = {
  storageKey: string
  defaultSidebarWidth: number
  minSidebarWidth: number
  maxSidebarWidth: number
  onResizeEnd?: () => void
}

export function useShellLayout({
  storageKey,
  defaultSidebarWidth,
  minSidebarWidth,
  maxSidebarWidth,
  onResizeEnd,
}: UseShellLayoutOptions) {
  const isMobile = useIsMobileLayout()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(
    () => window.localStorage.getItem(storageKey) === '1',
  )
  const [sidebarWidth, setSidebarWidth] = useState(defaultSidebarWidth)

  useEffect(() => {
    window.localStorage.setItem(storageKey, sidebarCollapsed ? '1' : '0')
  }, [sidebarCollapsed, storageKey])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setSidebarOpen(false)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])

  const shellStyle = useMemo(
    () =>
      ({
        '--sidebar-width': `${sidebarCollapsed ? 0 : sidebarWidth}px`,
      }) as CSSProperties,
    [sidebarCollapsed, sidebarWidth],
  )

  const layoutGridClass = useMemo(
    () =>
      sidebarCollapsed
        ? 'grid h-full grid-cols-[1fr] grid-rows-[1fr] md:[grid-template-columns:48px_1fr]'
        : 'grid h-full grid-cols-[1fr] grid-rows-[1fr] md:[grid-template-columns:48px_var(--sidebar-width)_6px_1fr]',
    [sidebarCollapsed],
  )

  const startSidebarResize = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      if (isMobile || sidebarCollapsed) {
        return
      }

      event.preventDefault()
      const startX = event.clientX
      const startWidth = sidebarWidth

      const onMove = (moveEvent: MouseEvent) => {
        const delta = moveEvent.clientX - startX
        const nextWidth = Math.min(
          maxSidebarWidth,
          Math.max(minSidebarWidth, startWidth + delta),
        )
        setSidebarWidth(nextWidth)
      }

      const onUp = () => {
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
        if (onResizeEnd) {
          window.requestAnimationFrame(onResizeEnd)
        }
      }

      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    },
    [
      isMobile,
      maxSidebarWidth,
      minSidebarWidth,
      onResizeEnd,
      sidebarCollapsed,
      sidebarWidth,
    ],
  )

  return {
    sidebarOpen,
    setSidebarOpen,
    sidebarCollapsed,
    setSidebarCollapsed,
    shellStyle,
    layoutGridClass,
    startSidebarResize,
  }
}
