import { useState } from 'react'
import { ChevronsLeft, ChevronsRight, Menu, Settings } from 'lucide-react'
import { Link, useRouterState } from '@tanstack/react-router'
import SettingsDialog from '@/components/settings/SettingsDialog'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type SideRailProps = {
  sidebarCollapsed: boolean
  onToggleSidebarCollapsed: () => void
  onToggleSidebarOpen: () => void
  showSidebarToggles?: boolean
}

export default function SideRail({
  sidebarCollapsed,
  onToggleSidebarCollapsed,
  onToggleSidebarOpen,
  showSidebarToggles = true,
}: SideRailProps) {
  const [settingsOpen, setSettingsOpen] = useState(false)

  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const tmuxActive = pathname === '/tmux'
  const servicesActive = pathname === '/services'
  const runbooksActive = pathname === '/runbooks'
  const alertsActive = pathname === '/alerts'
  const timelineActive = pathname === '/timeline'
  const metricsActive = pathname === '/metrics'

  const navItemClass = (isActive: boolean) =>
    cn(
      'grid h-7 w-8 place-items-center rounded-md border text-[11px] no-underline',
      isActive
        ? 'border-primary/40 bg-primary/15 text-primary-text-bright'
        : 'border-transparent text-secondary-foreground hover:border-border hover:bg-accent hover:text-foreground',
    )

  return (
    <aside className="hidden md:flex flex-col items-center gap-2 border-r border-border-subtle bg-surface-raised px-1.5 py-2">
      <TooltipHelper content="Sentinel home" side="right">
        <a
          className="brand-logo grid h-8 w-8 place-items-center rounded-md border border-border text-[12px] font-bold text-primary-text-light no-underline"
          href="/"
          aria-label="Sentinel home"
        >
          S
        </a>
      </TooltipHelper>
      <TooltipHelper content="Tmux" side="right">
        <Link className={navItemClass(tmuxActive)} to="/tmux" aria-label="Tmux">
          TM
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Services" side="right">
        <Link
          className={navItemClass(servicesActive)}
          to="/services"
          aria-label="Services"
        >
          SV
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Runbooks" side="right">
        <Link
          className={navItemClass(runbooksActive)}
          to="/runbooks"
          aria-label="Runbooks"
        >
          RB
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Alerts" side="right">
        <Link className={navItemClass(alertsActive)} to="/alerts" aria-label="Alerts">
          AL
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Timeline" side="right">
        <Link
          className={navItemClass(timelineActive)}
          to="/timeline"
          aria-label="Timeline"
        >
          TL
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Metrics" side="right">
        <Link
          className={navItemClass(metricsActive)}
          to="/metrics"
          aria-label="Metrics"
        >
          MT
        </Link>
      </TooltipHelper>
      <div className="flex-1" />
      <TooltipHelper content="Settings" side="right">
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-8 text-secondary-foreground hover:text-foreground"
          onClick={() => setSettingsOpen(true)}
          aria-label="Settings"
        >
          <Settings className="h-4 w-4" />
        </Button>
      </TooltipHelper>
      {showSidebarToggles && (
        <>
          <TooltipHelper
            content={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            side="right"
          >
            <Button
              variant="ghost"
              size="icon"
              className="hidden h-7 w-8 text-secondary-foreground hover:text-foreground md:grid"
              onClick={onToggleSidebarCollapsed}
              aria-label={
                sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'
              }
            >
              {sidebarCollapsed ? (
                <ChevronsRight className="h-4 w-4" />
              ) : (
                <ChevronsLeft className="h-4 w-4" />
              )}
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Open menu" side="right">
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-8 text-secondary-foreground hover:text-foreground md:hidden"
              onClick={onToggleSidebarOpen}
              aria-label="Open menu"
            >
              <Menu className="h-4 w-4" />
            </Button>
          </TooltipHelper>
        </>
      )}
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </aside>
  )
}
