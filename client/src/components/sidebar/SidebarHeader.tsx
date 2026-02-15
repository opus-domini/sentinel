import { Lock, LockOpen, Plus } from 'lucide-react'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'

type SidebarHeaderProps = {
  title: string
  count: number
  hasToken: boolean
  lockTitle: string
  canCreate: boolean
  onToggleAdd: () => void
  onToggleLock: () => void
}

export default function SidebarHeader({
  title,
  count,
  hasToken,
  lockTitle,
  canCreate,
  onToggleAdd,
  onToggleLock,
}: SidebarHeaderProps) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
        {title}
      </span>
      <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
        {count}
      </span>
      <div className="ml-auto flex items-center gap-1.5">
        <TooltipHelper
          content={canCreate ? 'New session' : 'tmux not available'}
        >
          <Button
            variant="ghost"
            size="icon"
            className="border border-border bg-surface-hover text-foreground hover:bg-accent"
            onClick={onToggleAdd}
            aria-label="New session"
            disabled={!canCreate}
          >
            <Plus className="h-4 w-4" />
          </Button>
        </TooltipHelper>
        <TooltipHelper content={lockTitle}>
          <Button
            variant="ghost"
            size="icon"
            className="border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
            onClick={onToggleLock}
            aria-label="API token"
          >
            {hasToken ? (
              <Lock className="h-4 w-4" />
            ) : (
              <LockOpen className="h-4 w-4" />
            )}
          </Button>
        </TooltipHelper>
      </div>
    </div>
  )
}
