import { AlertTriangle, CalendarClock, CheckCircle2, Play } from 'lucide-react'
import type { OpsRunbook, OpsRunbookRun, OpsSchedule } from '@/types'
import {
  isActiveRunbookJob,
  runbookStatusMeta,
} from '@/lib/runbookPresentation'
import { cn } from '@/lib/utils'

type RunbookOperationsSummaryProps = {
  runbooks: Array<OpsRunbook>
  jobs: Array<OpsRunbookRun>
  schedules: Array<OpsSchedule>
}

function nextScheduleLabel(schedules: Array<OpsSchedule>): string {
  const next = schedules
    .filter((schedule) => schedule.enabled && schedule.nextRunAt)
    .slice()
    .sort((a, b) => Date.parse(a.nextRunAt) - Date.parse(b.nextRunAt))[0]
  if (!next) {
    return 'No upcoming schedule'
  }
  const parsed = Date.parse(next.nextRunAt)
  if (Number.isNaN(parsed)) {
    return next.name
  }
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: next.timezone || undefined,
  }).format(new Date(parsed))
}

export function RunbookOperationsSummary({
  runbooks,
  jobs,
  schedules,
}: RunbookOperationsSummaryProps) {
  const activeJobs = jobs.filter(isActiveRunbookJob)
  const failedRunbooks = runbooks.filter(
    (runbook) => runbookStatusMeta(runbook, jobs).tone === 'danger',
  )
  const healthyRunbooks = runbooks.filter(
    (runbook) => runbookStatusMeta(runbook, jobs).tone === 'ok',
  )
  const scheduledRunbooks = new Set(
    schedules
      .filter((schedule) => schedule.enabled)
      .map((schedule) => schedule.runbookId),
  )
  const mostRecentJob = jobs
    .slice()
    .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))[0]
  const mostRecentRunbook = mostRecentJob
    ? runbooks.find((runbook) => runbook.id === mostRecentJob.runbookId)
    : null
  const mostRecentStatus =
    mostRecentRunbook != null
      ? runbookStatusMeta(mostRecentRunbook, jobs)
      : null

  const items = [
    {
      label: 'Active runs',
      value: activeJobs.length,
      detail:
        activeJobs[0]?.currentStep ||
        activeJobs[0]?.runbookName ||
        'Execution queue is idle',
      icon: Play,
      className:
        activeJobs.length > 0
          ? 'text-warning-foreground'
          : 'text-muted-foreground',
    },
    {
      label: 'Failed last runs',
      value: failedRunbooks.length,
      detail: failedRunbooks[0]?.name ?? 'No failed runbooks',
      icon: AlertTriangle,
      className:
        failedRunbooks.length > 0
          ? 'text-destructive-foreground'
          : 'text-muted-foreground',
    },
    {
      label: 'Scheduled',
      value: scheduledRunbooks.size,
      detail: nextScheduleLabel(schedules),
      icon: CalendarClock,
      className: 'text-muted-foreground',
    },
    {
      label: 'Recent health',
      value: healthyRunbooks.length,
      detail: mostRecentStatus
        ? `${mostRecentJob?.runbookName}: ${mostRecentStatus.label}`
        : 'No run history',
      icon: CheckCircle2,
      className: mostRecentStatus?.textClass ?? 'text-muted-foreground',
    },
  ]

  return (
    <section className="grid gap-2 md:grid-cols-4">
      {items.map((item) => {
        const Icon = item.icon
        return (
          <div
            key={item.label}
            className="grid min-w-0 gap-1 rounded-lg border border-border-subtle bg-secondary px-2.5 py-2"
          >
            <div className="flex min-w-0 items-center gap-1.5">
              <Icon className={cn('h-3.5 w-3.5 shrink-0', item.className)} />
              <span className="min-w-0 flex-1 truncate text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                {item.label}
              </span>
              <span className={cn('text-[13px] font-semibold', item.className)}>
                {item.value}
              </span>
            </div>
            <p className="truncate text-[11px] text-muted-foreground">
              {item.detail}
            </p>
          </div>
        )
      })}
    </section>
  )
}
