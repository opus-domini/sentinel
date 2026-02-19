import type { ConnectionState } from '@/types'
import { connectionDotClass, connectionLabel } from '@/lib/connection'
import { TooltipHelper } from '@/components/TooltipHelper'

type ConnectionBadgeProps = {
  state: ConnectionState
}

export default function ConnectionBadge({ state }: ConnectionBadgeProps) {
  const label = connectionLabel(state)
  return (
    <TooltipHelper content={label}>
      <span
        className="inline-flex h-4 w-4 items-center justify-center rounded-full border border-border-subtle bg-surface-elevated"
        role="status"
        aria-label={label}
      >
        <span
          className={`inline-block h-2 w-2 rounded-full ${connectionDotClass(state)}`}
        />
      </span>
    </TooltipHelper>
  )
}
