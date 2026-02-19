import {
  Activity,
  Bell,
  Blocks,
  ChevronsLeft,
  ChevronsRight,
  Clock,
  ScrollText,
  Settings,
  Shield,
  SquareTerminal,
} from 'lucide-react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { cn } from '@/lib/utils'

type SideRailProps = {
  sidebarCollapsed: boolean
  onToggleSidebarCollapsed: () => void
  showSidebarToggles?: boolean
}

export default function SideRail({
  sidebarCollapsed,
  onToggleSidebarCollapsed,
  showSidebarToggles = true,
}: SideRailProps) {
  const { setSettingsOpen } = useLayoutContext()

  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const tmuxActive = pathname === '/tmux'
  const servicesActive = pathname === '/services'
  const runbooksActive = pathname === '/runbooks'
  const alertsActive = pathname === '/alerts'
  const activitiesActive = pathname === '/activities'
  const metricsActive = pathname === '/metrics'
  const guardrailsActive = pathname === '/guardrails'

  const navItemClass = (isActive: boolean) =>
    cn(
      'relative flex h-9 w-full items-center justify-center rounded-md no-underline',
      isActive
        ? 'bg-primary/10 text-primary-text-bright before:absolute before:inset-y-1 before:left-0 before:w-0.5 before:rounded-full before:bg-primary'
        : 'text-secondary-foreground hover:bg-accent hover:text-foreground',
    )

  const navItems = [
    {
      to: '/tmux' as const,
      label: 'Tmux',
      active: tmuxActive,
      Icon: SquareTerminal,
    },
    {
      to: '/services' as const,
      label: 'Services',
      active: servicesActive,
      Icon: Blocks,
    },
    {
      to: '/alerts' as const,
      label: 'Alerts',
      active: alertsActive,
      Icon: Bell,
    },
    {
      to: '/metrics' as const,
      label: 'Metrics',
      active: metricsActive,
      Icon: Activity,
    },
    {
      to: '/runbooks' as const,
      label: 'Runbooks',
      active: runbooksActive,
      Icon: ScrollText,
    },
    {
      to: '/activities' as const,
      label: 'Activities',
      active: activitiesActive,
      Icon: Clock,
    },
    {
      to: '/guardrails' as const,
      label: 'Guardrails',
      active: guardrailsActive,
      Icon: Shield,
    },
  ]

  return (
    <aside className="hidden w-12 flex-col gap-1 border-r border-border-subtle bg-surface-raised px-1 py-2 md:flex">
      {navItems.map(({ to, label, active, Icon }) => (
        <TooltipHelper key={to} content={label} side="right">
          <Link
            className={navItemClass(active)}
            to={to}
            aria-label={label}
            aria-current={active ? 'page' : undefined}
          >
            <Icon className="size-4" />
          </Link>
        </TooltipHelper>
      ))}
      <div className="flex-1" />
      <hr className="w-full border-t border-border-subtle" />
      <TooltipHelper content="Settings" side="right">
        <Button
          variant="ghost"
          size="icon-lg"
          className="w-full text-secondary-foreground hover:text-foreground"
          onClick={() => setSettingsOpen(true)}
          aria-label="Settings"
        >
          <Settings className="size-4" />
        </Button>
      </TooltipHelper>
      {showSidebarToggles && (
        <TooltipHelper
          content={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          side="right"
        >
          <Button
            variant="ghost"
            size="icon-lg"
            className="hidden w-full text-secondary-foreground hover:text-foreground md:flex"
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
      )}
    </aside>
  )
}
