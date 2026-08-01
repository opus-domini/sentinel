import { Hourglass } from 'lucide-react'

import type { SessionLifecycle } from '@/types'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useDateFormat } from '@/hooks/useDateFormat'
import { cn } from '@/lib/utils'

type SessionLifecycleIndicatorProps = {
  lifecycle: SessionLifecycle
  className?: string
}

const cleanupStateLabels: Record<SessionLifecycle['cleanupState'], string> = {
  active: 'Active',
  grace: 'Cleanup grace period',
  cleanup_blocked: 'Cleanup blocked by an attached client',
}

export function formatLifecycleRemaining(deadline: string, now = Date.now()): string {
  const deadlineMs = Date.parse(deadline)
  if (Number.isNaN(deadlineMs)) return 'deadline unavailable'

  const remainingMinutes = Math.ceil((deadlineMs - now) / 60_000)
  if (remainingMinutes <= 0) return 'deadline reached'
  if (remainingMinutes < 60) {
    return `${remainingMinutes} minute${remainingMinutes === 1 ? '' : 's'} remaining`
  }

  const hours = Math.floor(remainingMinutes / 60)
  const minutes = remainingMinutes % 60
  const hourText = `${hours} hour${hours === 1 ? '' : 's'}`
  if (minutes === 0) return `${hourText} remaining`
  return `${hourText} ${minutes} minute${minutes === 1 ? '' : 's'} remaining`
}

export default function SessionLifecycleIndicator({
  lifecycle,
  className,
}: SessionLifecycleIndicatorProps) {
  const { formatTimestamp } = useDateFormat()
  const deadline =
    lifecycle.cleanupState === 'active'
      ? lifecycle.expiresAt
      : lifecycle.graceUntil || lifecycle.expiresAt
  const stateLabel = cleanupStateLabels[lifecycle.cleanupState]
  const deadlineLabel = formatTimestamp(deadline)
  const remainingLabel = formatLifecycleRemaining(deadline)
  const accessibleLabel = `Ephemeral MCP session, ${stateLabel.toLowerCase()}, ${remainingLabel}, deadline ${deadlineLabel}`
  const tooltip = [
    'Ephemeral MCP session',
    `State: ${stateLabel}`,
    `Deadline: ${deadlineLabel}`,
    `Remaining: ${remainingLabel}`,
  ].join('\n')

  return (
    <TooltipHelper content={tooltip}>
      <span
        className={cn(
          'inline-flex shrink-0 items-center justify-center',
          lifecycle.cleanupState === 'active' && 'text-primary',
          lifecycle.cleanupState === 'grace' && 'text-warning-foreground',
          lifecycle.cleanupState === 'cleanup_blocked' && 'text-destructive-foreground',
          className,
        )}
        role="img"
        aria-label={accessibleLabel}
        data-session-lifecycle={lifecycle.cleanupState}
      >
        <Hourglass className="size-full" aria-hidden="true" />
      </span>
    </TooltipHelper>
  )
}
