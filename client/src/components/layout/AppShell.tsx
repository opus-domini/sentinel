import { useCallback } from 'react'
import SideRail from '../SideRail'
import type { ReactNode } from 'react'

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
    shellStyle,
    layoutGridClass,
    startSidebarResize,
  } = useLayoutContext()

  const isMobile = useIsMobileLayout()

  const handleSwipeOpen = useCallback(() => {
    setSidebarOpen(true)
  }, [setSidebarOpen])

  useEdgeSwipe({
    enabled: isMobile,
    isOpen: sidebarOpen,
    onSwipeOpen: handleSwipeOpen,
  })

  return (
    <div className="h-dvh overflow-hidden bg-background text-foreground">
      <div className={layoutGridClass} style={shellStyle}>
        <SideRail
          sidebarCollapsed={sidebarCollapsed}
          onToggleSidebarCollapsed={() => setSidebarCollapsed((prev) => !prev)}
          onToggleSidebarOpen={() => setSidebarOpen((prev) => !prev)}
        />

        {sidebar}

        {!sidebarCollapsed && (
          <div
            className="hidden cursor-col-resize border-r border-border-subtle hover:bg-primary/20 md:block"
            onMouseDown={startSidebarResize}
          />
        )}

        {children}
      </div>

      <div
        className={cn(
          'fixed inset-0 z-20 bg-black/45 md:hidden',
          sidebarOpen ? 'block' : 'hidden',
        )}
        onClick={() => setSidebarOpen(false)}
      />
    </div>
  )
}
