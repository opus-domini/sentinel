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

  if (onClick) {
    return (
      <TooltipHelper content={tooltip}>
        <button
          type="button"
          className={cn(
            'inline-flex h-4 w-4 cursor-pointer items-center justify-center rounded-full border border-border-subtle bg-surface-elevated hover:bg-surface-active',
          )}
          aria-label={`${label}; ${actionLabel}`}
          onClick={onClick}
        >
          {dot}
        </button>
      </TooltipHelper>
    )
  }

  return (
    <TooltipHelper content={tooltip}>
      <span
        className="inline-flex h-4 w-4 items-center justify-center rounded-full border border-border-subtle bg-surface-elevated"
        role="status"
        aria-label={label}
      >
        {dot}
      </span>
    </TooltipHelper>
  )
}
