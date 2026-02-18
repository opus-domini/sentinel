import { useCallback, useMemo, useState } from 'react'
import cronstrue from 'cronstrue'
import { CronExpressionParser } from 'cron-parser'
import { Clock, Save, Trash2, X } from 'lucide-react'
import type { OpsSchedule } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

export type ScheduleDraft = {
  name: string
  scheduleType: 'cron' | 'once'
  cronExpr: string
  timezone: string
  runAt: string
  enabled: boolean
}

type RunbookScheduleEditorProps = {
  runbookId: string
  schedule: OpsSchedule | null
  saving: boolean
  onSave: (data: ScheduleDraft) => void
  onCancel: () => void
  onDelete?: () => void
}

const CRON_PRESETS = [
  { label: 'Every hour', value: '0 * * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Daily at midnight', value: '0 0 * * *' },
  { label: 'Daily at 9 AM', value: '0 9 * * *' },
  { label: 'Weekdays at 9 AM', value: '0 9 * * 1-5' },
  { label: 'Weekly on Monday', value: '0 0 * * 1' },
  { label: 'Custom', value: 'custom' },
] as const

const TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Asia/Tokyo',
  'Asia/Shanghai',
  'Australia/Sydney',
] as const

function scheduleToPreset(cronExpr: string): string {
  const match = CRON_PRESETS.find(
    (p) => p.value !== 'custom' && p.value === cronExpr,
  )
  return match ? match.value : 'custom'
}

function formatNextRun(date: Date, timezone: string): string {
  try {
    return new Intl.DateTimeFormat('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: timezone,
      timeZoneName: 'short',
    }).format(date)
  } catch {
    return date.toLocaleString()
  }
}

function initDraft(schedule: OpsSchedule | null): ScheduleDraft {
  if (schedule) {
    return {
      name: schedule.name,
      scheduleType: schedule.scheduleType as 'cron' | 'once',
      cronExpr: schedule.cronExpr,
      timezone: schedule.timezone || 'UTC',
      runAt: schedule.runAt,
      enabled: schedule.enabled,
    }
  }
  return {
    name: '',
    scheduleType: 'cron',
    cronExpr: '0 9 * * 1-5',
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
    runAt: '',
    enabled: true,
  }
}

export function RunbookScheduleEditor({
  schedule,
  saving,
  onSave,
  onCancel,
  onDelete,
}: RunbookScheduleEditorProps) {
  const isEditing = schedule != null

  const [draft, setDraft] = useState<ScheduleDraft>(() => initDraft(schedule))
  const [selectedPreset, setSelectedPreset] = useState<string>(() =>
    scheduleToPreset(draft.cronExpr),
  )

  const updateField = useCallback(
    <TKey extends keyof ScheduleDraft>(
      key: TKey,
      value: ScheduleDraft[TKey],
    ) => {
      setDraft((prev) => ({ ...prev, [key]: value }))
    },
    [],
  )

  const handlePresetChange = useCallback(
    (value: string) => {
      setSelectedPreset(value)
      if (value !== 'custom') {
        updateField('cronExpr', value)
      }
    },
    [updateField],
  )

  const cronDescription = useMemo(() => {
    if (draft.scheduleType !== 'cron' || !draft.cronExpr.trim()) return null
    try {
      return cronstrue.toString(draft.cronExpr, { verbose: true })
    } catch {
      return null
    }
  }, [draft.scheduleType, draft.cronExpr])

  const nextRuns = useMemo(() => {
    if (draft.scheduleType !== 'cron' || !draft.cronExpr.trim()) return []
    try {
      const interval = CronExpressionParser.parse(draft.cronExpr, {
        tz: draft.timezone,
      })
      const runs: Array<string> = []
      for (let i = 0; i < 3; i++) {
        const next = interval.next()
        runs.push(formatNextRun(next.toDate(), draft.timezone))
      }
      return runs
    } catch {
      return []
    }
  }, [draft.scheduleType, draft.cronExpr, draft.timezone])

  const cronError = useMemo(() => {
    if (draft.scheduleType !== 'cron' || !draft.cronExpr.trim()) return null
    try {
      CronExpressionParser.parse(draft.cronExpr)
      return null
    } catch {
      return 'Invalid cron expression'
    }
  }, [draft.scheduleType, draft.cronExpr])

  const handleSave = useCallback(() => {
    onSave(draft)
  }, [draft, onSave])

  return (
    <div className="grid gap-3 rounded-lg border border-border-subtle bg-surface-elevated p-3">
      <div className="flex items-center gap-2">
        <Clock className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-[12px] font-semibold">
          {isEditing ? 'Edit Schedule' : 'New Schedule'}
        </span>
      </div>

      <div className="grid gap-2.5">
        {/* Name */}
        <div>
          <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Name
          </Label>
          <Input
            className="mt-0.5 h-8 bg-surface-overlay text-[12px]"
            placeholder="Schedule name"
            value={draft.name}
            onChange={(e) => updateField('name', e.target.value)}
          />
        </div>

        {/* Schedule Type */}
        <div>
          <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Type
          </Label>
          <div className="mt-1 flex items-center gap-4">
            <label className="flex cursor-pointer items-center gap-1.5 text-[12px] select-none">
              <input
                type="radio"
                name="scheduleType"
                value="cron"
                checked={draft.scheduleType === 'cron'}
                onChange={() => updateField('scheduleType', 'cron')}
                className="accent-primary"
              />
              <span>Recurring</span>
            </label>
            <label className="flex cursor-pointer items-center gap-1.5 text-[12px] select-none">
              <input
                type="radio"
                name="scheduleType"
                value="once"
                checked={draft.scheduleType === 'once'}
                onChange={() => updateField('scheduleType', 'once')}
                className="accent-primary"
              />
              <span>One-time</span>
            </label>
          </div>
        </div>

        {/* Recurring fields */}
        {draft.scheduleType === 'cron' && (
          <>
            <div>
              <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Preset
              </Label>
              <div className="mt-0.5">
                <Select
                  value={selectedPreset}
                  onValueChange={handlePresetChange}
                >
                  <SelectTrigger className="w-full bg-surface-overlay text-[12px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CRON_PRESETS.map((preset) => (
                      <SelectItem key={preset.value} value={preset.value}>
                        {preset.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div>
              <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Cron Expression
              </Label>
              <Input
                className={cn(
                  'mt-0.5 h-8 bg-surface-overlay font-mono text-[12px]',
                  cronError && 'border-red-500',
                )}
                placeholder="0 9 * * 1-5"
                value={draft.cronExpr}
                readOnly={selectedPreset !== 'custom'}
                onChange={(e) => {
                  updateField('cronExpr', e.target.value)
                  setSelectedPreset('custom')
                }}
              />
              {cronError && (
                <p className="mt-0.5 text-[10px] text-red-400">{cronError}</p>
              )}
            </div>

            {cronDescription && (
              <div className="rounded border border-border-subtle bg-surface-overlay px-2.5 py-1.5">
                <p className="text-[11px] text-muted-foreground">
                  {cronDescription}
                </p>
                {nextRuns.length > 0 && (
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    <span className="font-medium">Next:</span>{' '}
                    {nextRuns.join(', ')}
                  </p>
                )}
              </div>
            )}
          </>
        )}

        {/* One-time fields */}
        {draft.scheduleType === 'once' && (
          <div>
            <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Date / Time
            </Label>
            <Input
              type="datetime-local"
              className="mt-0.5 h-8 bg-surface-overlay text-[12px]"
              value={draft.runAt ? draft.runAt.slice(0, 16) : ''}
              onChange={(e) => {
                const value = e.target.value
                updateField('runAt', value ? new Date(value).toISOString() : '')
              }}
            />
          </div>
        )}

        {/* Timezone */}
        <div>
          <Label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Timezone
          </Label>
          <div className="mt-0.5">
            <Select
              value={draft.timezone}
              onValueChange={(v) => updateField('timezone', v)}
            >
              <SelectTrigger className="w-full bg-surface-overlay text-[12px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TIMEZONES.map((tz) => (
                  <SelectItem key={tz} value={tz}>
                    {tz}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Enabled */}
        <label className="flex cursor-pointer items-center gap-2 text-[12px] select-none">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(e) => updateField('enabled', e.target.checked)}
            className="h-3.5 w-3.5 rounded border-border accent-primary"
          />
          <span className="text-muted-foreground">Enabled</span>
        </label>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1.5 border-t border-border-subtle pt-2.5">
        <Button
          variant="outline"
          size="sm"
          className="h-7 cursor-pointer gap-1 px-3 text-[11px]"
          disabled={saving}
          onClick={handleSave}
        >
          <Save className="h-3 w-3" />
          {saving ? 'Saving...' : 'Save'}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-7 cursor-pointer gap-1 px-3 text-[11px]"
          onClick={onCancel}
        >
          <X className="h-3 w-3" />
          Cancel
        </Button>
        {isEditing && onDelete && (
          <Button
            variant="outline"
            size="sm"
            className="h-7 cursor-pointer gap-1 px-3 text-[11px] text-red-400 hover:text-red-300"
            onClick={onDelete}
          >
            <Trash2 className="h-3 w-3" />
            Delete
          </Button>
        )}
      </div>
    </div>
  )
}
