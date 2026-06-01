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
    <div className="h-dvh overflow-hidden bg-background pb-16 text-foreground md:pb-0">
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
      className="fixed inset-x-2 bottom-2 z-10 grid grid-cols-6 gap-1 rounded-2xl border border-border-subtle bg-surface-raised/95 p-1 shadow-2xl shadow-black/30 backdrop-blur md:hidden"
    >
      {PRIMARY_NAV_ITEMS.map(({ to, label, Icon }) => {
        const active = pathname === to
        return (
          <Link
            key={to}
            to={to}
            aria-label={label}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'grid min-w-0 place-items-center gap-0.5 rounded-xl px-1 py-1.5 text-[10px] no-underline transition-colors',
              active
                ? 'bg-primary/15 text-primary-text-bright ring-1 ring-primary/30'
                : 'text-secondary-foreground hover:bg-accent hover:text-foreground',
            )}
          >
            <Icon className="size-4" aria-hidden="true" />
            <span className="max-w-full truncate">{label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
