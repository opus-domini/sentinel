import { AlertTriangle, CalendarClock, CheckCircle2, ChevronRight, Pause, Play } from 'lucide-react'
import type { OpsRunbook, OpsRunbookRun, OpsSchedule } from '@/types'
import { TooltipHelper } from '@/components/TooltipHelper'
import {
  isExecutingRunbookJob,
  isWaitingApprovalRunbookJob,
  runbookStatusMeta,
} from '@/lib/runbookPresentation'
import { cn } from '@/lib/utils'

type RunbookOperationsSummaryProps = {
  runbooks: Array<OpsRunbook>
  jobs: Array<OpsRunbookRun>
  schedules: Array<OpsSchedule>
  selectedRunbookId: string | null
  onSelectRunbook: (runbookId: string) => void
}

function newestJob(a: OpsRunbookRun, b: OpsRunbookRun): number {
  return Date.parse(b.createdAt) - Date.parse(a.createdAt)
}

function enabledSchedules(schedules: Array<OpsSchedule>): Array<OpsSchedule> {
  return schedules.filter((schedule) => schedule.enabled)
}

function nextSchedule(schedules: Array<OpsSchedule>): OpsSchedule | null {
  const enabled = enabledSchedules(schedules)
  return (
    enabled
      .filter((schedule) => schedule.nextRunAt)
      .slice()
      .sort((a, b) => Date.parse(a.nextRunAt) - Date.parse(b.nextRunAt))[0] ??
    enabled[0] ??
    null
  )
}

function nextScheduleLabel(schedules: Array<OpsSchedule>): string {
  const next = nextSchedule(schedules)
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
  selectedRunbookId,
  onSelectRunbook,
}: RunbookOperationsSummaryProps) {
  const activeJobs = jobs.filter(isExecutingRunbookJob).slice().sort(newestJob)
  const approvalJobs = jobs.filter(isWaitingApprovalRunbookJob).slice().sort(newestJob)
  const failedRunbooks = runbooks.filter(
    (runbook) => runbookStatusMeta(runbook, jobs).tone === 'danger',
  )
  const healthyRunbooks = runbooks.filter(
    (runbook) => runbookStatusMeta(runbook, jobs).tone === 'ok',
  )
  const activeSchedule = nextSchedule(schedules)
  const scheduledRunbooks = new Set(
    enabledSchedules(schedules).map((schedule) => schedule.runbookId),
  )

  const items = [
    {
      label: 'Active runs',
      shortLabel: 'Active',
      value: activeJobs.length,
      detail: activeJobs[0]?.currentStep || activeJobs[0]?.runbookName || 'Execution queue is idle',
      targetRunbookId: activeJobs[0]?.runbookId ?? null,
      icon: Play,
      className: activeJobs.length > 0 ? 'text-warning-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Pending approvals',
      shortLabel: 'Approvals',
      value: approvalJobs.length,
      detail:
        approvalJobs[0]?.currentStep || approvalJobs[0]?.runbookName || 'No approval gates waiting',
      targetRunbookId: approvalJobs[0]?.runbookId ?? null,
      icon: Pause,
      className: approvalJobs.length > 0 ? 'text-warning-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Failed last runs',
      shortLabel: 'Failed',
      value: failedRunbooks.length,
      detail: failedRunbooks[0]?.name ?? 'No failed runbooks',
      targetRunbookId: failedRunbooks[0]?.id ?? null,
      icon: AlertTriangle,
      className:
        failedRunbooks.length > 0 ? 'text-destructive-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Scheduled',
      shortLabel: 'Scheduled',
      value: scheduledRunbooks.size,
      detail: nextScheduleLabel(schedules),
      targetRunbookId: activeSchedule?.runbookId ?? null,
      icon: CalendarClock,
      className: 'text-muted-foreground',
    },
    {
      label: 'Healthy runbooks',
      shortLabel: 'Health',
      value: healthyRunbooks.length,
      detail: healthyRunbooks[0]?.name ?? 'No healthy runbooks',
      targetRunbookId: healthyRunbooks[0]?.id ?? null,
      icon: CheckCircle2,
      className: healthyRunbooks.length > 0 ? 'text-ok-foreground' : 'text-muted-foreground',
    },
  ]

  return (
    <section
      className="grid min-w-0 grid-cols-2 gap-1.5 [&>*:last-child]:col-span-2 sm:grid-cols-5 sm:[&>*:last-child]:col-span-1"
      aria-label="Runbook operations summary"
    >
      {items.map((item) => {
        const Icon = item.icon
        const isActionable = item.targetRunbookId != null
        const isSelected = item.targetRunbookId === selectedRunbookId
        return (
          <TooltipHelper key={item.label} side="bottom" content={`${item.label}\n${item.detail}`}>
            <button
              type="button"
              aria-disabled={!isActionable}
              tabIndex={isActionable ? 0 : -1}
              onClick={() => {
                if (item.targetRunbookId != null) {
                  onSelectRunbook(item.targetRunbookId)
                }
              }}
              className={cn(
                'group h-12 min-w-0 overflow-hidden rounded-md border border-border-subtle bg-secondary px-2 py-1.5 text-left transition-colors',
                isActionable &&
                  'cursor-pointer hover:border-accent hover:bg-accent/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                isSelected && 'border-accent bg-accent/15',
                !isActionable && 'cursor-default opacity-80',
              )}
              aria-label={`${item.label}: ${item.value}. ${item.detail}`}
            >
              <div className="flex min-w-0 items-center gap-1.5">
                <Icon className={cn('h-3.5 w-3.5 shrink-0', item.className)} />
                <span className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-[0.06em] text-muted-foreground">
                  {item.shortLabel}
                </span>
                <span className={cn('shrink-0 text-[13px] font-semibold', item.className)}>
                  {item.value}
                </span>
              </div>
              <div className="mt-0.5 flex min-w-0 items-center gap-1">
                <p className="min-w-0 flex-1 truncate text-[10px] text-muted-foreground">
                  {item.detail}
                </p>
                {isActionable && (
                  <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                )}
              </div>
            </button>
          </TooltipHelper>
        )
      })}
    </section>
  )
}
