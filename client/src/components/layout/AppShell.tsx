import { useCallback } from 'react'
import SideRail from '../SideRail'
import type { ReactNode } from 'react'
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
    settingsOpen,
    setSettingsOpen,
    shellStyle,
    layoutGridClass,
    startSidebarResize,
  } = useLayoutContext()

  const hasSidebar = sidebar != null
  const effectiveCollapsed = !hasSidebar || sidebarCollapsed
  const gridClass = effectiveCollapsed
    ? 'grid h-full grid-cols-[1fr] grid-rows-[1fr] md:[grid-template-columns:48px_1fr]'
    : layoutGridClass

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
      <div className={gridClass} style={shellStyle}>
        <SideRail
          sidebarCollapsed={sidebarCollapsed}
          onToggleSidebarCollapsed={() => setSidebarCollapsed((prev) => !prev)}
        />

        {hasSidebar && sidebar}

        {hasSidebar && !sidebarCollapsed && (
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
        role="presentation"
        aria-label="Close sidebar"
        onClick={() => setSidebarOpen(false)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setSidebarOpen(false)
          }
        }}
      />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </div>
  )
}
