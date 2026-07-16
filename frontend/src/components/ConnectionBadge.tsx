import type { ConnectionState } from '@/types'
import { connectionDotClass, connectionLabel } from '@/lib/connection'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type ConnectionBadgeProps = {
  state: ConnectionState
  detail?: string
  onClick?: () => void
  actionLabel?: string
}

export default function ConnectionBadge({
  state,
  detail,
  onClick,
  actionLabel = 'Resync connection',
}: ConnectionBadgeProps) {
  const label = connectionLabel(state)
  const base = detail && state !== 'connected' ? `${label} — ${detail}` : label
  const tooltip = onClick ? `${base} — ${actionLabel}` : base
  const dot = <span className={`inline-block h-2 w-2 rounded-full ${connectionDotClass(state)}`} />
  const content = (
    <>
      {dot}
      {state === 'error' && <span className="text-[11px] font-medium">{label}</span>}
    </>
  )

  if (onClick) {
    return (
      <TooltipHelper content={tooltip}>
        <button
          type="button"
          className={cn(
            'inline-flex h-7 cursor-pointer items-center justify-center gap-1.5 rounded-full border border-border-subtle bg-surface-elevated hover:bg-surface-active',
            state === 'error' ? 'px-2 text-destructive-foreground' : 'w-7',
          )}
          aria-label={`${label}; ${actionLabel}`}
          onClick={onClick}
        >
          {content}
        </button>
      </TooltipHelper>
    )
  }

  return (
    <TooltipHelper content={tooltip}>
      <span
        className={cn(
          'inline-flex h-7 items-center justify-center gap-1.5 rounded-full border border-border-subtle bg-surface-elevated',
          state === 'error' ? 'px-2 text-destructive-foreground' : 'w-7',
        )}
        role="status"
        aria-label={label}
      >
        {content}
      </span>
    </TooltipHelper>
  )
}
