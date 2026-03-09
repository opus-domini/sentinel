import cronstrue from 'cronstrue'
import { Clock, Pause, Pencil, Play, Trash2 } from 'lucide-react'
import type { OpsRunbook, OpsSchedule } from '@/types'
import type { ScheduleDraft } from '@/components/RunbookScheduleEditor'
import { RunbookScheduleEditor } from '@/components/RunbookScheduleEditor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'

function isBuiltinRunbook(id: string): boolean {
  return id.startsWith('ops.')
}

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
  schedule: OpsSchedule | null
  editingSchedule: {
    runbookId: string
    schedule: OpsSchedule | null
  } | null
  scheduleSaving: boolean
  onEdit: (runbook: OpsRunbook) => void
  onDelete: (runbook: OpsRunbook) => void
  onRun: (runbookId: string) => void
  onEditSchedule: (value: {
    runbookId: string
    schedule: OpsSchedule | null
  }) => void
  onCancelScheduleEdit: () => void
  onSaveSchedule: (draft: ScheduleDraft) => void
  onDeleteSchedule: (scheduleId: string) => void
  onToggleScheduleEnabled: (schedule: OpsSchedule) => void
  onTriggerSchedule: (scheduleId: string) => void
}

export function RunbookDetailPanel({
  runbook,
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
  return (
    <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-3">
      <div className="grid gap-2 sm:flex sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="truncate text-[14px] font-semibold">{runbook.name}</h2>
          <p className="text-[12px] text-muted-foreground">
            {runbook.description}
          </p>
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
          {isBuiltinRunbook(runbook.id) ? (
            <TooltipHelper content="Built-in runbooks cannot be deleted">
              <span>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 gap-1 px-3 text-[11px]"
                  disabled
                >
                  <Trash2 className="h-3 w-3" />
                  Delete
                </Button>
              </span>
            </TooltipHelper>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="h-8 cursor-pointer gap-1 px-3 text-[11px] text-red-400 hover:text-red-300"
              onClick={() => onDelete(runbook)}
            >
              <Trash2 className="h-3 w-3" />
              Delete
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            className="h-8 cursor-pointer gap-1 px-3 text-[11px]"
            onClick={() => onRun(runbook.id)}
          >
            <Play className="h-3 w-3" />
            Run
          </Button>
        </div>
      </div>

      <div className="grid gap-1">
        <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
          Steps ({runbook.steps.length})
        </p>
        {runbook.steps.map((step, i) => (
          <div
            key={i}
            className="flex items-start gap-2 rounded border border-border-subtle bg-surface-overlay px-2 py-1.5 text-[11px]"
          >
            <span className="mt-0.5 shrink-0 rounded bg-surface-elevated px-1 text-[10px] text-muted-foreground">
              {i + 1}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="rounded border border-border-subtle px-1 text-[9px] uppercase text-muted-foreground">
                  {step.type}
                </span>
                <span className="font-medium">{step.title}</span>
              </div>
              {step.command && (
                <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                  {step.command}
                </p>
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
                <Badge
                  variant="outline"
                  className="h-4 px-1 text-[9px] text-amber-400"
                >
                  required
                </Badge>
              )}
              {param.label && (
                <span className="text-muted-foreground">{param.label}</span>
              )}
              {param.default && (
                <span className="text-muted-foreground">= {param.default}</span>
              )}
              {param.type === 'select' &&
                param.options &&
                param.options.length > 0 && (
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
              <span className="text-[11px] font-medium">
                {scheduleDescription(schedule)}
              </span>
              <span className="text-[10px] text-muted-foreground">
                ({schedule.timezone})
              </span>
              {!schedule.enabled && (
                <span className="rounded bg-amber-500/20 px-1 text-[9px] font-medium text-amber-400">
                  paused
                </span>
              )}
            </div>
            {schedule.nextRunAt && (
              <p className="mt-0.5 text-[10px] text-muted-foreground">
                Next:{' '}
                {formatScheduleDate(schedule.nextRunAt, schedule.timezone)}
              </p>
            )}
            {schedule.lastRunAt && (
              <p className="text-[10px] text-muted-foreground">
                Last:{' '}
                {formatScheduleDate(schedule.lastRunAt, schedule.timezone)}
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
                    schedule: schedule,
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
                className="h-6 cursor-pointer gap-1 px-2 text-[10px] text-red-400 hover:text-red-300"
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
