import { useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  Bell,
  Blocks,
  Clock,
  ScrollText,
  Settings,
  SquareTerminal,
  X,
} from 'lucide-react'
import type { ReactNode } from 'react'
import SettingsDialog from '@/components/settings/SettingsDialog'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { cn } from '@/lib/utils'

type SidebarShellProps = {
  isOpen: boolean
  collapsed: boolean
  children: ReactNode
  widthClassName?: string
}

const mobileNavItems = [
  { to: '/tmux', label: 'Tmux', icon: SquareTerminal },
  { to: '/services', label: 'Services', icon: Blocks },
  { to: '/runbooks', label: 'Runbooks', icon: ScrollText },
  { to: '/alerts', label: 'Alerts', icon: Bell },
  { to: '/timeline', label: 'Timeline', icon: Clock },
  { to: '/metrics', label: 'Metrics', icon: Activity },
] as const

function MobileNav() {
  const { setSidebarOpen } = useLayoutContext()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  const navItemClass = (isActive: boolean) =>
    cn(
      'relative grid size-8 place-items-center rounded-md no-underline',
      isActive
        ? 'bg-primary/10 text-primary-text-bright before:absolute before:inset-x-1 before:-bottom-2 before:h-0.5 before:rounded-full before:bg-primary'
        : 'text-secondary-foreground hover:bg-accent hover:text-foreground',
    )

  const handleNav = () => setSidebarOpen(false)

  return (
    <div className="flex items-center justify-between border-b border-border pb-2 md:hidden">
      <div className="flex items-center gap-1">
        {mobileNavItems.map(({ to, label, icon: Icon }) => (
          <Link
            key={to}
            className={navItemClass(pathname === to)}
            to={to}
            onClick={handleNav}
            aria-label={label}
          >
            <Icon className="size-4" />
          </Link>
        ))}
      </div>
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="size-7 text-secondary-foreground hover:text-foreground"
          onClick={() => setSettingsOpen(true)}
          aria-label="Settings"
        >
          <Settings className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-7 text-secondary-foreground hover:text-foreground"
          onClick={() => setSidebarOpen(false)}
          aria-label="Close menu"
        >
          <X className="size-4" />
        </Button>
      </div>
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </div>
  )
}

export default function SidebarShell({
  isOpen,
  collapsed,
  children,
  widthClassName = 'w-[min(85vw,320px)]',
}: SidebarShellProps) {
  return (
    <aside
      className={cn(
        'fixed left-0 top-0 z-30 h-screen border-r border-border bg-card p-2 transition-transform duration-200 ease-out md:static md:z-auto md:h-full md:min-h-0 md:w-auto md:min-w-0 md:overflow-hidden md:translate-x-0 md:transition-none',
        widthClassName,
        collapsed ? 'md:hidden' : 'md:block',
        isOpen ? 'translate-x-0' : '-translate-x-[108%]',
      )}
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      <MobileNav />
      {children}
    </aside>
  )
}
