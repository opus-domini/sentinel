import { Lock, LockOpen, Plus } from 'lucide-react'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'

type SidebarHeaderProps = {
  title: string
  count: number
  hasToken: boolean
  lockTitle: string
  canCreate: boolean
  helpDialog?: React.ReactNode
  addControl?: React.ReactNode
  onToggleAdd: () => void
  onToggleLock: () => void
}

export default function SidebarHeader({
  title,
  count,
  hasToken,
  lockTitle,
  canCreate,
  helpDialog,
  addControl,
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
      <div className="ml-auto flex items-center gap-1">
        {helpDialog}
        {addControl ?? (
          <TooltipHelper
            content={canCreate ? 'New session' : 'tmux not available'}
          >
            <Button
              variant="outline"
              size="icon-xs"
              className="text-foreground"
              onClick={onToggleAdd}
              aria-label="New session"
              disabled={!canCreate}
            >
              <Plus className="h-3 w-3" />
            </Button>
          </TooltipHelper>
        )}
        <TooltipHelper content={lockTitle}>
          <Button
            variant="outline"
            size="icon-xs"
            className="text-secondary-foreground"
            onClick={onToggleLock}
            aria-label="API token"
          >
            {hasToken ? (
              <Lock className="h-3 w-3" />
            ) : (
              <LockOpen className="h-3 w-3" />
            )}
          </Button>
        </TooltipHelper>
      </div>
    </div>
  )
}
