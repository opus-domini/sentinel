import { Link } from '@tanstack/react-router'
import { Activity, Blocks, ScrollText, ShieldCheck, SquareTerminal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { NowSnapshot } from '@/types'
import { useDateFormat } from '@/hooks/useDateFormat'
import {
  NOW_SOURCE_ORDER,
  nowSourceLabel,
  nowSourceStatusLabel,
  presentNowPosture,
} from '@/lib/nowPresentation'
import type { NowSourceName } from '@/lib/nowPresentation'
import { cn } from '@/lib/utils'

const sourceMeta: Record<
  NowSourceName,
  { to: '/services' | '/metrics' | '/runbooks' | '/tmux'; Icon: LucideIcon }
> = {
  services: { to: '/services', Icon: Blocks },
  metrics: { to: '/metrics', Icon: Activity },
  runbooks: { to: '/runbooks', Icon: ScrollText },
  tmux: { to: '/tmux', Icon: SquareTerminal },
}

function sourceTone(status: string): string {
  if (status === 'current') return 'border-ok/30 bg-ok/8 text-ok-foreground'
  if (status === 'stale') return 'border-warning/35 bg-warning/8 text-warning-foreground'
  if (status === 'not_configured') {
    return 'border-border-subtle bg-surface-overlay text-muted-foreground'
  }
  return 'border-destructive/35 bg-destructive/8 text-destructive-foreground'
}

type NowStatusProps = {
  snapshot: NowSnapshot
}

export function NowStatus({ snapshot }: NowStatusProps) {
  const { formatRelativeTime } = useDateFormat()
  const presentation = presentNowPosture(snapshot.posture.state)
  const postureTone =
    presentation.tone === 'ok'
      ? 'text-ok-foreground'
      : presentation.tone === 'warning'
        ? 'text-warning-foreground'
        : 'text-destructive-foreground'

  return (
    <section
      aria-labelledby="now-status-title"
      className="relative overflow-hidden rounded-xl border border-border-subtle bg-[linear-gradient(118deg,rgba(36,200,242,0.08),transparent_36%),var(--surface-raised)]"
    >
      <div className="grid gap-5 p-4 sm:p-5 lg:grid-cols-[minmax(15rem,0.9fr)_minmax(28rem,1.8fr)] lg:items-center">
        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2 text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
            <ShieldCheck className="size-3.5 text-primary" />
            <span id="now-status-title">Host posture</span>
          </div>
          <p
            className={cn(
              'text-[clamp(1.75rem,4vw,3.4rem)] font-semibold leading-none',
              postureTone,
            )}
          >
            {presentation.label}
          </p>
          <p className="mt-2 max-w-[36rem] text-[11px] leading-relaxed text-secondary-foreground sm:text-[12px]">
            {presentation.detail}
          </p>
          <p className="mt-3 text-[10px] text-muted-foreground">
            Confidence{' '}
            <span className="font-medium text-secondary-foreground">
              {snapshot.confidence.state === 'current' ? 'Current' : 'Degraded'}
            </span>{' '}
            · Snapshot {formatRelativeTime(snapshot.generatedAt)}
          </p>
        </div>

        <div className="relative grid grid-cols-2 gap-2 lg:grid-cols-4">
          {NOW_SOURCE_ORDER.map((name) => {
            const source = snapshot.confidence.sources[name]
            const { to, Icon } = sourceMeta[name]
            return (
              <Link
                key={name}
                to={to}
                className={cn(
                  'group relative z-[1] grid min-w-0 gap-2 rounded-lg border px-3 py-2.5 no-underline transition-colors hover:bg-surface-hover focus-visible:ring-2 focus-visible:ring-ring',
                  sourceTone(source.status),
                )}
                aria-label={`${nowSourceLabel(name)}: ${nowSourceStatusLabel(source.status)}`}
                title={source.message}
              >
                <div className="flex items-center justify-between gap-2">
                  <Icon className="size-3.5" />
                  <span className="size-1.5 rounded-full bg-current shadow-[0_0_8px_currentColor]" />
                </div>
                <div className="min-w-0">
                  <p className="truncate text-[11px] font-semibold text-foreground">
                    {nowSourceLabel(name)}
                  </p>
                  <p className="truncate text-[9px] uppercase tracking-[0.08em]">
                    {nowSourceStatusLabel(source.status)}
                  </p>
                  <p className="mt-0.5 truncate text-[9px] text-muted-foreground">
                    Observed {formatRelativeTime(source.observedAt)}
                  </p>
                </div>
              </Link>
            )
          })}
        </div>
      </div>

      <div className="grid grid-cols-2 border-t border-border-subtle bg-surface-inset/65 sm:grid-cols-5">
        {[
          ['Tracked', snapshot.posture.services.tracked],
          ['Running', snapshot.posture.services.running],
          ['Failed', snapshot.posture.services.failed],
          ['Inactive', snapshot.posture.services.inactive],
          ['Metric signals', snapshot.posture.metrics.signals.length],
        ].map(([label, value]) => (
          <div
            key={label}
            className="flex items-baseline justify-between gap-2 border-r border-border-subtle px-3 py-2 last:border-r-0 sm:block"
          >
            <span className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
              {label}
            </span>
            <p className="text-[13px] font-semibold text-foreground sm:mt-0.5">{value}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
