import { Link } from '@tanstack/react-router'
import { Settings } from 'lucide-react'

import { TooltipHelper } from '@/components/TooltipHelper'
import { useViewport } from '@/contexts/ViewportContext'
import { cn } from '@/lib/utils'

type MobileSettingsLinkProps = {
  className?: string
}

export default function MobileSettingsLink({ className }: MobileSettingsLinkProps) {
  const { compactLayout } = useViewport()
  if (!compactLayout) return null
  return (
    <TooltipHelper content="Settings" side="bottom">
      <Link
        to="/settings"
        aria-label="Settings"
        className={cn(
          'grid size-11 shrink-0 place-items-center rounded-md text-secondary-foreground no-underline hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring',
          className,
        )}
      >
        <Settings className="size-4" aria-hidden="true" />
      </Link>
    </TooltipHelper>
  )
}
