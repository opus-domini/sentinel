import { FocusScope } from '@radix-ui/react-focus-scope'
import { Settings, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useViewport } from '@/contexts/ViewportContext'
import { cn } from '@/lib/utils'

type SidebarShellProps = {
  isOpen: boolean
  collapsed: boolean
  children: ReactNode
  widthClassName?: string
}

function MobileNav() {
  const { setSidebarOpen, setSettingsOpen } = useLayoutContext()
  const { compactLayout } = useViewport()

  if (!compactLayout) {
    return null
  }

  // Section navigation lives in the bottom tab bar; the drawer is just the
  // master list, so it only needs Settings and Close here (no duplicate nav).
  return (
    <div className="flex items-center justify-end gap-1 border-b border-border pb-2">
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
