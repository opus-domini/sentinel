import cronstrue from 'cronstrue'
import {
  CheckCircle2,
  Clock,
  Pause,
  Pencil,
  Play,
  SlidersHorizontal,
  Timer,
  Trash2,
  Webhook,
  XCircle,
} from 'lucide-react'
import type { OpsRunbook, OpsRunbookRun, OpsRunbookStep, OpsSchedule } from '@/types'
import type { ScheduleDraft } from '@/components/RunbookScheduleEditor'
import { RunbookScheduleEditor } from '@/components/RunbookScheduleEditor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useDateFormat } from '@/hooks/useDateFormat'
import { cn } from '@/lib/utils'
import {
  formatRunbookDuration,
  isActiveRunbookJob,
  isWaitingApprovalRunbookJob,
  runbookJobDurationMs,
  runbookJobProgress,
  runbookStatusMeta,
} from '@/lib/runbookPresentation'

function scheduleDescription(schedule: OpsSchedule): string {
  if (schedule.scheduleType === 'cron' && schedule.cronExpr) {
    try {
      return cronstrue.toString(schedule.cronExpr)
    } catch {
      return schedule.cronExpr
    }
  }
  if (schedule.scheduleType === 'once' && schedule.runAt) {
    return `Once at ${new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: schedule.timezone || undefined }).format(new Date(schedule.runAt))}`
  }
  return schedule.name
}

function runbookStepKey(step: OpsRunbookStep): string {
  return [
    step.type,
    step.title,
    step.command ?? '',
    step.script ?? '',
    step.description ?? '',
    step.continueOnError ? 'continue' : 'stop',
    step.timeout ?? '',
    step.retries ?? '',
    step.retryDelay ?? '',
  ].join(':')
}

function runbookStepItems(steps: Array<OpsRunbookStep>) {
  const seen = new Map<string, number>()

  return steps.map((step, position) => {
    const baseKey = runbookStepKey(step)
    const occurrence = seen.get(baseKey) ?? 0
    seen.set(baseKey, occurrence + 1)

    return {
      key: occurrence === 0 ? baseKey : `${baseKey}:${occurrence + 1}`,
      position,
      step,
    }
  })
}

function formatScheduleDate(iso: string, tz?: string): string {
  if (!iso) return ''
  try {
    return new Intl.DateTimeFormat('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: tz || undefined,
      timeZoneName: 'short',
    }).format(new Date(iso))
  } catch {
    return iso
  }
}

type RunbookDetailPanelProps = {
  runbook: OpsRunbook
  lastJob: OpsRunbookRun | null
  schedule: OpsSchedule | null
  editingSchedule: {
    runbookId: string
    schedule: OpsSchedule | null
  } | null
  scheduleSaving: boolean
  onEdit: (runbook: OpsRunbook) => void
  onDelete: (runbook: OpsRunbook) => void
  onRun: (runbookId: string) => void
  onEditSchedule: (value: { runbookId: string; schedule: OpsSchedule | null }) => void
  onCancelScheduleEdit: () => void
  onSaveSchedule: (draft: ScheduleDraft) => void
  onDeleteSchedule: (scheduleId: string) => void
  onToggleScheduleEnabled: (schedule: OpsSchedule) => void
  onTriggerSchedule: (scheduleId: string) => void
}

export function RunbookDetailPanel({
  runbook,
  lastJob,
  schedule,
  editingSchedule,
  scheduleSaving,
  onEdit,
  onDelete,
  onRun,
  onEditSchedule,
  onCancelScheduleEdit,
  onSaveSchedule,
  onDeleteSchedule,
  onToggleScheduleEnabled,
  onTriggerSchedule,
}: RunbookDetailPanelProps) {
  const { formatDateTime } = useDateFormat()
  const status = runbookStatusMeta(runbook, lastJob ? [lastJob] : [])
  const activeRun = lastJob != null && isActiveRunbookJob(lastJob)
  const progress = lastJob ? runbookJobProgress(lastJob) : 0
  const waitingApproval = lastJob != null && isWaitingApprovalRunbookJob(lastJob)
  const lastRunStatusLabel = waitingApproval ? 'Waiting approval' : (lastJob?.status ?? '')
  const runDuration = lastJob ? formatRunbookDuration(runbookJobDurationMs(lastJob)) : 'n/a'
  const lastRunLabel = lastJob
    ? `${lastRunStatusLabel} · ${formatDateTime(lastJob.createdAt)}`
    : 'No runs recorded'
  const scheduleLabel = schedule
    ? schedule.enabled
      ? schedule.nextRunAt
        ? `Next ${formatScheduleDate(schedule.nextRunAt, schedule.timezone)}`
        : 'Enabled'
      : 'Paused'
    : 'Manual only'

  return (
    <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-3">
      <div className="grid gap-2 sm:flex sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="truncate text-[14px] font-semibold">{runbook.name}</h2>
          <p className="text-[12px] text-muted-foreground">{runbook.description}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Button
            variant="outline"
            size="sm"
            className="h-8 cursor-pointer gap-1 px-3 text-[11px]"
            onClick={() => onEdit(runbook)}
          >
            <Pencil className="h-3 w-3" />
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-8 cursor-pointer gap-1 px-3 text-[11px] text-destructive-foreground hover:text-destructive-foreground"
            onClick={() => onDelete(runbook)}
          >
            <Trash2 className="h-3 w-3" />
            Delete
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-8 cursor-pointer gap-1 px-3 text-[11px]"
            onClick={() => onRun(runbook.id)}
          >
            <Play className="h-3 w-3" />
            {activeRun ? 'Run again' : 'Run'}
          </Button>
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <div className="grid gap-1 rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
          <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            Readiness
          </span>
          <span
            className={cn(
              'inline-flex min-w-0 items-center gap-1.5 text-[12px] font-semibold',
              status.textClass,
            )}
          >
            <span className={cn('h-2 w-2 rounded-full', status.dotClass)} />
            {status.label}
          </span>
        </div>
        <div className="grid gap-1 rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
          <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            Last run
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-[12px]">
            {lastJob == null ? (
              <Clock className="h-3 w-3 shrink-0 text-muted-foreground" />
            ) : waitingApproval ? (
              <Pause className="h-3 w-3 shrink-0 text-warning-foreground" />
            ) : lastJob.status.trim().toLowerCase() === 'failed' ? (
              <XCircle className="h-3 w-3 shrink-0 text-destructive-foreground" />
            ) : (
              <CheckCircle2 className="h-3 w-3 shrink-0 text-ok-foreground" />
            )}
            <span className="min-w-0 truncate">{lastRunLabel}</span>
          </span>
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <Timer className="h-2.5 w-2.5" />
            {runDuration}
          </span>
          {activeRun && (
            <span className="h-1 overflow-hidden rounded-full bg-surface-elevated">
              <span
                className="block h-full rounded-full bg-warning"
                style={{ width: `${progress}%` }}
              />
            </span>
          )}
        </div>
        <div className="grid gap-1 rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
          <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            Schedule
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-[12px]">
            <Clock className="h-3 w-3 shrink-0 text-muted-foreground" />
            <span className="min-w-0 truncate">{scheduleLabel}</span>
          </span>
        </div>
        <div className="grid gap-1 rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
          <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            Inputs
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-[12px]">
            <SlidersHorizontal className="h-3 w-3 shrink-0 text-muted-foreground" />
            <span>{runbook.parameters?.length ?? 0} parameters</span>
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-muted-foreground">
            <Webhook className="h-2.5 w-2.5 shrink-0" />
            <span className="min-w-0 truncate">
              {runbook.webhookURL ? 'Webhook enabled' : 'No webhook'}
            </span>
          </span>
        </div>
      </div>

      <div className="grid gap-1">
        <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
          Steps ({runbook.steps.length})
        </p>
        {runbookStepItems(runbook.steps).map(({ key, position, step }) => (
          <div
            key={key}
            className="flex items-start gap-2 rounded border border-border-subtle bg-surface-overlay px-2 py-1.5 text-[11px]"
          >
            <span className="mt-0.5 shrink-0 rounded bg-surface-elevated px-1 text-[10px] text-muted-foreground">
              {position + 1}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="rounded border border-border-subtle px-1 text-[9px] uppercase text-muted-foreground">
                  {step.type}
                </span>
                <span className="font-medium">{step.title}</span>
              </div>
              {step.type === 'run' && step.command && (
                <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                  {step.command}
                </p>
              )}
              {step.type === 'script' && step.script && (
                <pre className="mt-0.5 max-h-20 overflow-hidden truncate whitespace-pre-wrap font-mono text-[10px] text-muted-foreground">
                  {step.script}
                </pre>
              )}
              {step.type === 'approval' && step.description && (
                <p className="mt-0.5 text-[10px] text-muted-foreground">{step.description}</p>
              )}
              {(step.continueOnError || step.timeout || step.retries) && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {step.continueOnError && (
                    <Badge variant="outline" className="h-4 px-1 text-[9px]">
                      continue on error
                    </Badge>
                  )}
                  {step.timeout != null && step.timeout > 0 && (
                    <Badge variant="outline" className="h-4 px-1 text-[9px]">
                      timeout: {step.timeout}s
                    </Badge>
                  )}
                  {step.retries != null && step.retries > 0 && (
                    <Badge variant="outline" className="h-4 px-1 text-[9px]">
                      retries: {step.retries}
                      {step.retryDelay != null && step.retryDelay > 0 && (
                        <span className="ml-0.5 text-muted-foreground">
                          ({step.retryDelay}s delay)
                        </span>
                      )}
                    </Badge>
                  )}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* Parameters section */}
      {runbook.parameters && runbook.parameters.length > 0 && (
        <div className="grid gap-1">
          <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Parameters ({runbook.parameters.length})
          </p>
          {runbook.parameters.map((param) => (
            <div
              key={param.name}
              className="flex items-center gap-2 rounded border border-border-subtle bg-surface-overlay px-2 py-1.5 text-[11px]"
            >
              <span className="font-mono font-medium">{param.name}</span>
              <Badge variant="outline" className="h-4 px-1 text-[9px]">
                {param.type}
              </Badge>
              {param.required && (
                <Badge variant="outline" className="h-4 px-1 text-[9px] text-warning-foreground">
                  required
                </Badge>
              )}
              {param.label && <span className="text-muted-foreground">{param.label}</span>}
              {param.default && <span className="text-muted-foreground">= {param.default}</span>}
              {param.type === 'select' && param.options && param.options.length > 0 && (
                <span className="truncate text-[10px] text-muted-foreground">
                  [{param.options.join(', ')}]
                </span>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Schedule section */}
      <div className="grid gap-1">
        <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
          Schedule
        </p>
        {editingSchedule != null && editingSchedule.runbookId === runbook.id ? (
          <RunbookScheduleEditor
            runbookId={runbook.id}
            schedule={editingSchedule.schedule}
            saving={scheduleSaving}
            onSave={(draft) => onSaveSchedule(draft)}
            onCancel={onCancelScheduleEdit}
            onDelete={
              editingSchedule.schedule
                ? () => onDeleteSchedule(editingSchedule.schedule!.id)
                : undefined
            }
          />
        ) : schedule ? (
          <div className="rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
            <div className="flex items-center gap-1.5">
              <Clock className="h-3 w-3 text-muted-foreground" />
              <span className="text-[11px] font-medium">{scheduleDescription(schedule)}</span>
              <span className="text-[10px] text-muted-foreground">({schedule.timezone})</span>
              {!schedule.enabled && (
                <span className="rounded bg-warning/20 px-1 text-[9px] font-medium text-warning-foreground">
                  paused
                </span>
              )}
            </div>
            {schedule.nextRunAt && (
              <p className="mt-0.5 text-[10px] text-muted-foreground">
                Next: {formatScheduleDate(schedule.nextRunAt, schedule.timezone)}
              </p>
            )}
            {schedule.lastRunAt && (
              <p className="text-[10px] text-muted-foreground">
                Last: {formatScheduleDate(schedule.lastRunAt, schedule.timezone)}
                {schedule.lastRunStatus && ` \u00b7 ${schedule.lastRunStatus}`}
              </p>
            )}
            <div className="mt-1.5 flex items-center gap-1">
              <Button
                variant="outline"
                size="sm"
                className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                onClick={() =>
                  onEditSchedule({
                    runbookId: runbook.id,
                    schedule,
                  })
                }
              >
                <Pencil className="h-2.5 w-2.5" />
                Edit
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                onClick={() => onToggleScheduleEnabled(schedule)}
              >
                {schedule.enabled ? (
                  <>
                    <Pause className="h-2.5 w-2.5" />
                    Pause
                  </>
                ) : (
                  <>
                    <Play className="h-2.5 w-2.5" />
                    Resume
                  </>
                )}
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                onClick={() => onTriggerSchedule(schedule.id)}
              >
                <Play className="h-2.5 w-2.5" />
                Trigger
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-6 cursor-pointer gap-1 px-2 text-[10px] text-destructive-foreground hover:text-destructive-foreground"
                onClick={() => onDeleteSchedule(schedule.id)}
              >
                <Trash2 className="h-2.5 w-2.5" />
                Delete
              </Button>
            </div>
          </div>
        ) : (
          <Button
            variant="outline"
            size="sm"
            className="h-7 w-fit cursor-pointer gap-1 px-3 text-[11px]"
            onClick={() =>
              onEditSchedule({
                runbookId: runbook.id,
                schedule: null,
              })
            }
          >
            <Clock className="h-3 w-3" />
            Add Schedule
          </Button>
        )}
      </div>
    </div>
  )
}
