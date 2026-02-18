import { useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Settings, X } from 'lucide-react'
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

function MobileNav() {
  const { setSidebarOpen } = useLayoutContext()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  const navItemClass = (isActive: boolean) =>
    cn(
      'grid h-7 w-8 place-items-center rounded-md border text-[11px] no-underline',
      isActive
        ? 'border-primary/40 bg-primary/15 text-primary-text-bright'
        : 'border-transparent text-secondary-foreground hover:border-border hover:bg-accent hover:text-foreground',
    )

  const handleNav = () => setSidebarOpen(false)

  return (
    <div className="flex items-center justify-between border-b border-border pb-2 md:hidden">
      <div className="flex items-center gap-1.5">
        <a
          className="brand-logo grid h-7 w-8 place-items-center rounded-md border border-border text-[11px] font-bold text-primary-text-light no-underline"
          href="/"
          onClick={handleNav}
          aria-label="Sentinel home"
        >
          S
        </a>
        <Link
          className={navItemClass(pathname === '/tmux')}
          to="/tmux"
          onClick={handleNav}
          aria-label="Tmux"
        >
          TM
        </Link>
        <Link
          className={navItemClass(pathname === '/services')}
          to="/services"
          onClick={handleNav}
          aria-label="Services"
        >
          SV
        </Link>
        <Link
          className={navItemClass(pathname === '/runbooks')}
          to="/runbooks"
          onClick={handleNav}
          aria-label="Runbooks"
        >
          RB
        </Link>
        <Link
          className={navItemClass(pathname === '/alerts')}
          to="/alerts"
          onClick={handleNav}
          aria-label="Alerts"
        >
          AL
        </Link>
        <Link
          className={navItemClass(pathname === '/timeline')}
          to="/timeline"
          onClick={handleNav}
          aria-label="Timeline"
        >
          TL
        </Link>
        <Link
          className={navItemClass(pathname === '/metrics')}
          to="/metrics"
          onClick={handleNav}
          aria-label="Metrics"
        >
          MT
        </Link>
      </div>
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-secondary-foreground hover:text-foreground"
          onClick={() => setSettingsOpen(true)}
          aria-label="Settings"
        >
          <Settings className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-secondary-foreground hover:text-foreground"
          onClick={() => setSidebarOpen(false)}
          aria-label="Close menu"
        >
          <X className="h-4 w-4" />
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
