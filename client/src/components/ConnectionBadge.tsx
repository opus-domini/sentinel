import type { ConnectionState } from '@/types'
import { connectionDotClass, connectionLabel } from '@/lib/connection'
import { TooltipHelper } from '@/components/TooltipHelper'

type ConnectionBadgeProps = {
  state: ConnectionState
}

export default function ConnectionBadge({ state }: ConnectionBadgeProps) {
  return (
    <TooltipHelper content={connectionLabel(state)}>
      <span
        className={`inline-block h-2 w-2 cursor-pointer rounded-full ${connectionDotClass(state)}`}
        aria-label={connectionLabel(state)}
      />
    </TooltipHelper>
  )
}
