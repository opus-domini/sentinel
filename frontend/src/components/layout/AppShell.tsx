import { useCallback } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import SideRail from '@/components/SideRail'
import type { KeyboardEvent, ReactNode } from 'react'
import SettingsDialog from '@/components/settings/SettingsDialog'

import { useLayoutContext } from '@/contexts/LayoutContext'
import { useEdgeSwipe } from '@/hooks/useEdgeSwipe'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { PRIMARY_NAV_ITEMS } from '@/lib/primaryNav'
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
    (event: KeyboardEvent<HTMLButtonElement>) => {
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
    <div className="app-shell h-full overflow-hidden bg-background text-foreground">
      <div className={gridClass} style={shellStyle}>
        <SideRail
          sidebarCollapsed={sidebarCollapsed}
          onToggleSidebarCollapsed={() => setSidebarCollapsed((prev) => !prev)}
          showSidebarToggles={hasSidebar}
        />

        {hasSidebar && sidebar}

        {hasSidebar && !sidebarCollapsed && (
          <button
            type="button"
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

      <MobilePrimaryNav />

      <button
        type="button"
        aria-label="Close sidebar"
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

function MobilePrimaryNav() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  return (
    <nav
      aria-label="Mobile primary navigation"
      className="mobile-primary-nav fixed inset-x-0 bottom-0 z-10 grid grid-cols-4 border-t border-border bg-background/95 px-1 pt-1 pb-0.5 backdrop-blur md:hidden"
    >
      {PRIMARY_NAV_ITEMS.map(({ to, label, shortLabel, Icon }) => {
        const active = pathname === to
        return (
          <Link
            key={to}
            to={to}
            aria-label={label}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'grid min-w-0 place-items-center gap-0 px-1 py-0.5 text-[9px] no-underline transition-colors',
              active
                ? 'text-primary/60 hover:text-primary/70'
                : 'text-secondary-foreground hover:bg-accent hover:text-foreground',
            )}
          >
            <Icon className="size-3.5 transition-colors" aria-hidden="true" />
            <span className="max-w-full truncate">{shortLabel ?? label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
