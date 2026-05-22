import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { useDocumentHotkeys } from '@/hooks/useHotkeys'
import type { SidebarDensity } from '@/contexts/LayoutContext'

type UseShellLayoutOptions = {
  storageKey: string
  defaultSidebarWidth: number
  minSidebarWidth: number
  maxSidebarWidth: number
  onResizeEnd?: () => void
}

function clampSidebarWidth(width: number, minWidth: number, maxWidth: number) {
  return Math.min(maxWidth, Math.max(minWidth, width))
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
      if (Number.isFinite(parsed) && parsed >= minSidebarWidth && parsed <= maxSidebarWidth) {
        return parsed
      }
    }
    return defaultSidebarWidth
  })
  const [settingsOpen, setSettingsOpen] = useState(false)

  const notifyResizeEnd = useCallback(() => {
    if (onResizeEnd) {
      window.requestAnimationFrame(onResizeEnd)
    }
  }, [onResizeEnd])

  const resizeSidebarTo = useCallback(
    (width: number) => {
      setSidebarWidth(clampSidebarWidth(width, minSidebarWidth, maxSidebarWidth))
      notifyResizeEnd()
    },
    [maxSidebarWidth, minSidebarWidth, notifyResizeEnd],
  )

  const resizeSidebarBy = useCallback(
    (delta: number) => {
      setSidebarWidth((current) =>
        clampSidebarWidth(current + delta, minSidebarWidth, maxSidebarWidth),
      )
      notifyResizeEnd()
    },
    [maxSidebarWidth, minSidebarWidth, notifyResizeEnd],
  )

  useEffect(() => {
    window.localStorage.setItem(storageKey, sidebarCollapsed ? '1' : '0')
  }, [sidebarCollapsed, storageKey])

  useEffect(() => {
    window.localStorage.setItem(widthStorageKey, String(sidebarWidth))
  }, [sidebarWidth, widthStorageKey])

  useDocumentHotkeys([
    {
      key: 'Escape',
      ignoreEditable: false,
      preventDefault: false,
      stopPropagation: false,
      when: (event) =>
        !(event.target instanceof Element && event.target.closest('[role="dialog"]')),
      handler: () => setSidebarOpen(false),
    },
    {
      key: '\\',
      ctrl: true,
      meta: false,
      alt: false,
      shift: false,
      allowTerminalTarget: true,
      handler: () => setSidebarCollapsed((current) => !current),
    },
  ])

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
    (event: ReactMouseEvent<HTMLElement>) => {
      if (isMobile || sidebarCollapsed) {
        return
      }

      event.preventDefault()
      const startX = event.clientX
      const startWidth = sidebarWidth

      const onMove = (moveEvent: MouseEvent) => {
        const delta = moveEvent.clientX - startX
        const nextWidth = clampSidebarWidth(startWidth + delta, minSidebarWidth, maxSidebarWidth)
        setSidebarWidth(nextWidth)
      }

      const onUp = () => {
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
        notifyResizeEnd()
      }

      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    },
    [isMobile, maxSidebarWidth, minSidebarWidth, notifyResizeEnd, sidebarCollapsed, sidebarWidth],
  )

  return {
    sidebarOpen,
    setSidebarOpen,
    sidebarCollapsed,
    setSidebarCollapsed,
    sidebarDensity,
    sidebarWidth,
    sidebarMinWidth: minSidebarWidth,
    sidebarMaxWidth: maxSidebarWidth,
    settingsOpen,
    setSettingsOpen,
    shellStyle,
    layoutGridClass,
    startSidebarResize,
    resizeSidebarBy,
    resizeSidebarTo,
  }
}
