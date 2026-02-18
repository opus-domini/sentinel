import { useState } from 'react'
import {
  Activity,
  Bell,
  Blocks,
  ChevronsLeft,
  ChevronsRight,
  Clock,
  Menu,
  ScrollText,
  Settings,
  SquareTerminal,
} from 'lucide-react'
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
      'relative grid size-8 place-items-center rounded-md no-underline',
      isActive
        ? 'bg-primary/10 text-primary-text-bright before:absolute before:inset-y-1 before:-left-2 before:w-0.5 before:rounded-full before:bg-primary'
        : 'text-secondary-foreground hover:bg-accent hover:text-foreground',
    )

  return (
    <aside className="hidden md:flex flex-col items-center gap-1 border-r border-border-subtle bg-surface-raised px-2 py-2">
      <TooltipHelper content="Tmux" side="right">
        <Link className={navItemClass(tmuxActive)} to="/tmux" aria-label="Tmux">
          <SquareTerminal className="size-4" />
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Services" side="right">
        <Link
          className={navItemClass(servicesActive)}
          to="/services"
          aria-label="Services"
        >
          <Blocks className="size-4" />
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Runbooks" side="right">
        <Link
          className={navItemClass(runbooksActive)}
          to="/runbooks"
          aria-label="Runbooks"
        >
          <ScrollText className="size-4" />
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Alerts" side="right">
        <Link
          className={navItemClass(alertsActive)}
          to="/alerts"
          aria-label="Alerts"
        >
          <Bell className="size-4" />
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Timeline" side="right">
        <Link
          className={navItemClass(timelineActive)}
          to="/timeline"
          aria-label="Timeline"
        >
          <Clock className="size-4" />
        </Link>
      </TooltipHelper>
      <TooltipHelper content="Metrics" side="right">
        <Link
          className={navItemClass(metricsActive)}
          to="/metrics"
          aria-label="Metrics"
        >
          <Activity className="size-4" />
        </Link>
      </TooltipHelper>
      <div className="flex-1" />
      <hr className="w-5 border-t border-border-subtle" />
      <TooltipHelper content="Settings" side="right">
        <Button
          variant="ghost"
          size="icon-lg"
          className="text-secondary-foreground hover:text-foreground"
          onClick={() => setSettingsOpen(true)}
          aria-label="Settings"
        >
          <Settings className="size-4" />
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
              size="icon-lg"
              className="hidden text-secondary-foreground hover:text-foreground md:grid"
              onClick={onToggleSidebarCollapsed}
              aria-label={
                sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'
              }
            >
              {sidebarCollapsed ? (
                <ChevronsRight className="size-4" />
              ) : (
                <ChevronsLeft className="size-4" />
              )}
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Open menu" side="right">
            <Button
              variant="ghost"
              size="icon-lg"
              className="text-secondary-foreground hover:text-foreground md:hidden"
              onClick={onToggleSidebarOpen}
              aria-label="Open menu"
            >
              <Menu className="size-4" />
            </Button>
          </TooltipHelper>
        </>
      )}
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </aside>
  )
}
