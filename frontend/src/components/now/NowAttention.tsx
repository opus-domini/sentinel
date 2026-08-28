import { Link } from '@tanstack/react-router'
import { Activity, ArrowUpRight, CircleAlert, Play, ShieldAlert } from 'lucide-react'
import type {
  MetricPostureSignal,
  NowAttention as NowAttentionData,
  NowServiceFailedAttention,
} from '@/types'
import { Button } from '@/components/ui/button'
import { useDateFormat } from '@/hooks/useDateFormat'
import { metricsSignalSearch, runbookExecutionSearch, serviceStatusSearch } from '@/lib/deepLinks'
import { presentMetricSignal } from '@/lib/metricsView'
import { nowAttentionHiddenCount } from '@/lib/nowPresentation'

type NowAttentionProps = {
  attention: NowAttentionData
  degraded: boolean
  onRunProcedure: (item: NowServiceFailedAttention) => void
}

export function primaryMetricSignal(
  signals: Array<MetricPostureSignal>,
): MetricPostureSignal | undefined {
  return signals.find((signal) => signal.severity === 'critical') ?? signals[0]
}

export function NowAttention({ attention, degraded, onRunProcedure }: NowAttentionProps) {
  const { formatRelativeTime } = useDateFormat()
  const hidden = nowAttentionHiddenCount(attention)

  return (
    <section
      aria-labelledby="now-attention-title"
      className="grid min-h-0 grid-rows-[auto_1fr_auto] overflow-hidden rounded-xl border border-border-subtle bg-secondary"
    >
      <header className="flex items-end justify-between gap-3 border-b border-border-subtle px-3 py-2.5 sm:px-4">
        <div>
          <p className="text-[9px] uppercase tracking-[0.13em] text-warning-foreground">
            Decision queue
          </p>
          <h2 id="now-attention-title" className="mt-0.5 text-[14px] font-semibold">
            Needs attention
          </h2>
        </div>
        <span className="rounded-full border border-border-subtle bg-surface-overlay px-2 py-0.5 text-[10px] text-secondary-foreground">
          {attention.total}
        </span>
      </header>

      <div className="min-h-0 overflow-y-auto p-2">
        {attention.visible.length === 0 ? (
          <div className="grid h-full min-h-36 place-items-center rounded-lg border border-dashed border-border-subtle bg-surface-sunken px-5 text-center">
            <div>
              <p className="text-[12px] font-medium text-foreground">
                {degraded ? 'No actionable item from available sources' : 'No action needed'}
              </p>
              <p className="mt-1 text-[10px] leading-relaxed text-muted-foreground">
                {degraded
                  ? 'Review the source state above before treating this snapshot as complete.'
                  : 'Current evidence does not require an operator decision.'}
              </p>
            </div>
          </div>
        ) : (
          <div className="grid gap-1.5">
            {attention.visible.map((item) => {
              if (item.type === 'runbook_approval') {
                return (
                  <Link
                    key={`${item.type}:${item.run.runId}`}
                    to="/runbooks"
                    search={runbookExecutionSearch(item.run.runId)}
                    className="group flex items-start gap-3 rounded-lg border border-warning/25 bg-warning/6 px-3 py-2.5 no-underline transition-colors hover:bg-warning/10"
                  >
                    <CircleAlert className="mt-0.5 size-4 shrink-0 text-warning-foreground" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-[12px] font-semibold text-foreground">
                        Approval waiting
                      </p>
                      <p className="truncate text-[10px] text-secondary-foreground">
                        {item.run.runbookName} · {formatRelativeTime(item.run.createdAt)}
                      </p>
                    </div>
                    <ArrowUpRight className="mt-0.5 size-3.5 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" />
                  </Link>
                )
              }

              if (item.type === 'service_failed') {
                return (
                  <div
                    key={`${item.type}:${item.service.name}`}
                    className="flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/7 px-3 py-2.5"
                  >
                    <ShieldAlert className="mt-0.5 size-4 shrink-0 text-destructive-foreground" />
                    <Link
                      to="/services"
                      search={serviceStatusSearch(item.service.name)}
                      className="group min-w-0 flex-1 no-underline"
                    >
                      <p className="truncate text-[12px] font-semibold text-foreground group-hover:text-primary-text">
                        {item.service.displayName || item.service.name} is failed
                      </p>
                      <p className="truncate text-[10px] text-secondary-foreground">
                        {item.service.scope} · {item.service.unit}
                      </p>
                    </Link>
                    {item.runbook && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 cursor-pointer border-primary/25 bg-primary/5 text-[10px] text-primary-text hover:bg-primary/10"
                        onClick={() => onRunProcedure(item)}
                      >
                        <Play className="size-3" />
                        Run procedure
                      </Button>
                    )}
                  </div>
                )
              }

              const primarySignal = primaryMetricSignal(item.signals)
              return (
                <Link
                  key={item.type}
                  to="/metrics"
                  search={
                    primarySignal
                      ? metricsSignalSearch(primarySignal.name, item.observedAt)
                      : undefined
                  }
                  className="group flex items-start gap-3 rounded-lg border border-warning/25 bg-warning/6 px-3 py-2.5 no-underline transition-colors hover:bg-warning/10"
                >
                  <Activity className="mt-0.5 size-4 shrink-0 text-warning-foreground" />
                  <div className="min-w-0 flex-1">
                    <p className="text-[12px] font-semibold text-foreground">
                      Host pressure is {item.severity}
                    </p>
                    <p className="truncate text-[10px] text-secondary-foreground">
                      {item.signals
                        .map((signal) => {
                          const label = presentMetricSignal(signal.name).label
                          return signal.subject ? `${label}: ${signal.subject}` : label
                        })
                        .join(' · ')}
                    </p>
                    <p className="mt-1 text-[9px] text-muted-foreground">
                      Observed {formatRelativeTime(item.observedAt)}
                    </p>
                  </div>
                  <ArrowUpRight className="mt-0.5 size-3.5 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" />
                </Link>
              )
            })}
          </div>
        )}
      </div>

      {hidden > 0 && (
        <footer className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-border-subtle px-3 py-2 text-[9px] text-muted-foreground sm:px-4">
          <span>{hidden} more in owner modules:</span>
          {attention.overflow.approvals > 0 && (
            <Link to="/runbooks" className="text-primary-text hover:underline">
              Runbooks {attention.overflow.approvals}
            </Link>
          )}
          {attention.overflow.services > 0 && (
            <Link to="/services" className="text-primary-text hover:underline">
              Services {attention.overflow.services}
            </Link>
          )}
          {attention.overflow.metrics > 0 && (
            <Link to="/metrics" className="text-primary-text hover:underline">
              Metrics {attention.overflow.metrics}
            </Link>
          )}
        </footer>
      )}
    </section>
  )
}
