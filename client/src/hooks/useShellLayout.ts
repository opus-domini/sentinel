import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import type { SidebarDensity } from '@/contexts/LayoutContext'

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
  const widthStorageKey = `${storageKey}_width`
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    const stored = window.localStorage.getItem(widthStorageKey)
    if (stored !== null) {
      const parsed = Number.parseFloat(stored)
      if (
        Number.isFinite(parsed) &&
        parsed >= minSidebarWidth &&
        parsed <= maxSidebarWidth
      ) {
        return parsed
      }
    }
    return defaultSidebarWidth
  })
  const [settingsOpen, setSettingsOpen] = useState(false)

  useEffect(() => {
    window.localStorage.setItem(storageKey, sidebarCollapsed ? '1' : '0')
  }, [sidebarCollapsed, storageKey])

  useEffect(() => {
    window.localStorage.setItem(widthStorageKey, String(sidebarWidth))
  }, [sidebarWidth, widthStorageKey])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        if ((event.target as Element).closest('[role="dialog"]')) return
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

  const sidebarDensity: SidebarDensity = sidebarCollapsed
    ? 'full'
    : sidebarWidth <= 250
      ? 'minimal'
      : sidebarWidth <= 300
        ? 'compact'
        : 'full'

  const layoutGridClass = useMemo(
    () =>
      sidebarCollapsed
        ? 'grid h-full grid-cols-[1fr] grid-rows-[minmax(0,1fr)] md:[grid-template-columns:48px_1fr]'
        : 'grid h-full grid-cols-[1fr] grid-rows-[minmax(0,1fr)] md:[grid-template-columns:48px_var(--sidebar-width)_6px_1fr]',
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
    sidebarDensity,
    settingsOpen,
    setSettingsOpen,
    shellStyle,
    layoutGridClass,
    startSidebarResize,
  }
}
