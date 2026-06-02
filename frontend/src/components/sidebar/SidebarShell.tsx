import { FocusScope } from '@radix-ui/react-focus-scope'
import { Settings, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { cn } from '@/lib/utils'

type SidebarShellProps = {
  isOpen: boolean
  collapsed: boolean
  children: ReactNode
  widthClassName?: string
}

function MobileNav() {
  const { setSidebarOpen, setSettingsOpen } = useLayoutContext()

  // Section navigation lives in the bottom tab bar; the drawer is just the
  // master list, so it only needs Settings and Close here (no duplicate nav).
  return (
    <div className="flex items-center justify-end gap-1 border-b border-border pb-2 md:hidden">
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
  )
}

export default function SidebarShell({
  isOpen,
  collapsed,
  children,
  widthClassName = 'w-[min(85vw,320px)]',
}: SidebarShellProps) {
  const isMobile = useIsMobileLayout()

  return (
    <aside
      aria-label="Session sidebar"
      className={cn(
        'fixed left-0 top-0 z-30 flex h-dvh flex-col overflow-hidden border-r border-border bg-card p-2 transition-transform duration-200 ease-out md:static md:z-auto md:h-full md:min-h-0 md:w-auto md:min-w-0 md:translate-x-0 md:transition-none',
        widthClassName,
        collapsed ? 'md:hidden' : 'md:flex',
        isOpen ? 'translate-x-0' : '-translate-x-[108%]',
      )}
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      <FocusScope
        className="flex min-h-0 flex-1 flex-col overflow-hidden outline-none"
        trapped={isMobile && isOpen}
        loop
      >
        <MobileNav />
        {children}
      </FocusScope>
    </aside>
  )
}
