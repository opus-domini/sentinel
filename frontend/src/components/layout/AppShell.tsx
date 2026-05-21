import { useCallback } from 'react'
import SideRail from '../SideRail'
import type { KeyboardEvent, ReactNode } from 'react'
import SettingsDialog from '@/components/settings/SettingsDialog'

import { useLayoutContext } from '@/contexts/LayoutContext'
import { useEdgeSwipe } from '@/hooks/useEdgeSwipe'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { cn } from '@/lib/utils'

type AppShellProps = {
  sidebar?: ReactNode
  children: ReactNode
}

export default function AppShell({ sidebar, children }: AppShellProps) {
  const {
    sidebarOpen,
    setSidebarOpen,
    sidebarCollapsed,
    setSidebarCollapsed,
    sidebarWidth,
    sidebarMinWidth,
    sidebarMaxWidth,
    settingsOpen,
    setSettingsOpen,
    shellStyle,
    layoutGridClass,
    startSidebarResize,
    resizeSidebarBy,
    resizeSidebarTo,
  } = useLayoutContext()

  const hasSidebar = sidebar != null
  const effectiveCollapsed = !hasSidebar || sidebarCollapsed
  const gridClass = effectiveCollapsed
    ? 'grid h-full grid-cols-[1fr] grid-rows-[minmax(0,1fr)] md:[grid-template-columns:48px_1fr]'
    : layoutGridClass

  const isMobile = useIsMobileLayout()

  const handleSwipeOpen = useCallback(() => {
    setSidebarOpen(true)
  }, [setSidebarOpen])

  const handleSidebarResizeKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (event.altKey || event.ctrlKey || event.metaKey) {
        return
      }

      const step = event.shiftKey ? 40 : 16
      if (event.key === 'ArrowLeft') {
        event.preventDefault()
        resizeSidebarBy(-step)
      } else if (event.key === 'ArrowRight') {
        event.preventDefault()
        resizeSidebarBy(step)
      } else if (event.key === 'Home') {
        event.preventDefault()
        resizeSidebarTo(sidebarMinWidth)
      } else if (event.key === 'End') {
        event.preventDefault()
        resizeSidebarTo(sidebarMaxWidth)
      }
    },
    [resizeSidebarBy, resizeSidebarTo, sidebarMaxWidth, sidebarMinWidth],
  )

  useEdgeSwipe({
    enabled: isMobile,
    isOpen: sidebarOpen,
    onSwipeOpen: handleSwipeOpen,
  })

  return (
    <div className="h-dvh overflow-hidden bg-background text-foreground">
      <div className={gridClass} style={shellStyle}>
        <SideRail
          sidebarCollapsed={sidebarCollapsed}
          onToggleSidebarCollapsed={() => setSidebarCollapsed((prev) => !prev)}
          showSidebarToggles={hasSidebar}
        />

        {hasSidebar && sidebar}

        {hasSidebar && !sidebarCollapsed && (
          <div
            role="separator"
            aria-label="Resize sidebar"
            aria-orientation="vertical"
            aria-valuemin={sidebarMinWidth}
            aria-valuemax={sidebarMaxWidth}
            aria-valuenow={Math.round(sidebarWidth)}
            tabIndex={0}
            className="hidden cursor-col-resize border-r border-border-subtle outline-none hover:bg-primary/20 focus-visible:bg-primary/25 focus-visible:ring-2 focus-visible:ring-ring md:block"
            onMouseDown={startSidebarResize}
            onKeyDown={handleSidebarResizeKeyDown}
          />
        )}

        {children}
      </div>

      <div
        className={cn(
          'fixed inset-0 z-20 bg-black/45 md:hidden',
          hasSidebar && sidebarOpen ? 'block' : 'hidden',
        )}
        onClick={() => setSidebarOpen(false)}
      />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </div>
  )
}
