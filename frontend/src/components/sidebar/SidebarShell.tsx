import { FocusScope } from '@radix-ui/react-focus-scope'
import { Settings, X } from 'lucide-react'
import { Link, useRouterState } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useViewport } from '@/contexts/ViewportContext'
import { cn } from '@/lib/utils'
import SidebarContextActions from './SidebarContextActions'

type SidebarShellProps = {
  isOpen: boolean
  collapsed: boolean
  children: ReactNode
  widthClassName?: string
}

function MobileNav() {
  const { setSidebarOpen } = useLayoutContext()
  const { compactLayout } = useViewport()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  if (!compactLayout) {
    return null
  }

  // Primary navigation lives in the bottom tab bar. The drawer header mirrors
  // the desktop rail's contextual actions and adds only its close control.
  return (
    <div className="flex items-center justify-start gap-0.5 border-b border-border pb-1.5">
      <Button
        variant="ghost"
        size="icon-lg"
        className="size-8 w-8 cursor-pointer touch-manipulation text-secondary-foreground hover:text-foreground"
        onClick={() => setSidebarOpen(false)}
        aria-label="Close menu"
      >
        <X className="size-4" />
      </Button>
      <TooltipHelper content="Settings" side="bottom">
        <Link
          to="/settings"
          aria-label="Settings"
          className="grid size-8 shrink-0 touch-manipulation place-items-center rounded-md text-secondary-foreground no-underline hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => setSidebarOpen(false)}
        >
          <Settings className="size-4" aria-hidden="true" />
        </Link>
      </TooltipHelper>
      <SidebarContextActions pathname={pathname} mobile />
    </div>
  )
}

export default function SidebarShell({
  isOpen,
  collapsed,
  children,
  widthClassName = 'w-[min(85vw,320px)]',
}: SidebarShellProps) {
  const { compactLayout } = useViewport()

  return (
    <aside
      aria-label="Session sidebar"
      className={cn(
        'flex flex-col overflow-hidden border-r border-border bg-card p-2',
        compactLayout
          ? 'fixed left-0 top-0 z-30 h-dvh transition-transform duration-200 ease-out'
          : 'static z-auto h-full min-h-0 w-auto min-w-0 translate-x-0 transition-none',
        compactLayout && widthClassName,
        !compactLayout && collapsed && 'hidden',
        compactLayout && (isOpen ? 'translate-x-0' : '-translate-x-[108%]'),
      )}
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      <FocusScope
        className="flex min-h-0 flex-1 flex-col overflow-hidden outline-none"
        trapped={compactLayout && isOpen}
        loop
      >
        <MobileNav />
        {children}
      </FocusScope>
    </aside>
  )
}
