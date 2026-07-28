import { useCallback, useEffect, useMemo, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { Activity, BookOpenCheck, FileText, Save, Undo2 } from 'lucide-react'

import {
  diffOperationsDraft,
  operationsDraftFromSettings,
  operationsPatchFromChanges,
  validateOperationsDraft,
} from './operationsDraft'
import type { OperationsDraft, OperationsDraftErrors, OperationsDraftKey } from './operationsDraft'
import RestartPendingNotice from './RestartPendingNotice'
import { SaveFeedback, SettingsField, SettingValueSummary, ValidationError } from './SettingsField'
import SettingsSectionHeader from './SettingsSectionHeader'
import type { BooleanSetting, IntegerSetting, StringSetting } from '@/api/settings'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToastContext } from '@/contexts/ToastContext'
import { useSettings } from '@/hooks/useSettings'
import { ApiError } from '@/hooks/useTmuxApi'
import { cn } from '@/lib/utils'

type EditorState = {
  base: OperationsDraft
  draft: OperationsDraft
}

type Feedback = {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}

const idleFeedback: Feedback = { status: 'idle', message: '' }

export default function OperationsSettings() {
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const operations = settings?.operations
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [errors, setErrors] = useState<OperationsDraftErrors>({})
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)

  useEffect(() => {
    if (settings == null) return
    const next = operationsDraftFromSettings(settings.operations)
    setEditor((current) => {
      if (current == null) {
        return { base: next, draft: next }
      }
      const pending = diffOperationsDraft(current.base, current.draft)
      if (pending.length === 0) return { base: next, draft: next }
      const preserved = { ...next }
      for (const change of pending) {
        preserveDraftValue(preserved, current.draft, change.key)
      }
      return { base: next, draft: preserved }
    })
  }, [settings])

  const changes = useMemo(
    () => (editor == null ? [] : diffOperationsDraft(editor.base, editor.draft)),
    [editor],
  )
  const dirty = changes.length > 0
  const shouldBlock = useCallback(
    ({ current, next }: { current: { pathname: string }; next: { pathname: string } }) =>
      dirty && current.pathname !== next.pathname,
    [dirty],
  )
  const blocker = useBlocker({
    shouldBlockFn: shouldBlock,
    enableBeforeUnload: dirty,
    withResolver: true,
  })

  const updateDraft = useCallback(
    <Key extends OperationsDraftKey>(key: Key, value: OperationsDraft[Key]) => {
      setEditor((current) =>
        current == null
          ? current
          : {
              ...current,
              draft: {
                ...current.draft,
                [key]: value,
              },
            },
      )
      setErrors((current) => ({ ...current, [key]: undefined }))
      setFeedback(idleFeedback)
    },
    [],
  )

  const discard = useCallback(() => {
    setEditor((current) =>
      current == null ? current : { base: current.base, draft: current.base },
    )
    setErrors({})
    setFeedback({ status: 'success', message: 'Unsaved changes discarded.' })
  }, [])

  const save = useCallback(async () => {
    if (editor == null || operations == null || changes.length === 0) return
    const nextErrors = validateOperationsDraft(editor.draft, operations)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      setFeedback({ status: 'error', message: 'Review the highlighted operational values.' })
      return
    }

    setFeedback({ status: 'saving', message: 'Saving operational settings…' })
    try {
      const snapshot = await settingsQuery.save(operationsPatchFromChanges(editor.draft, changes))
      const next = operationsDraftFromSettings(snapshot.settings.operations)
      setEditor({ base: next, draft: next })
      setErrors({})
      setFeedback({
        status: 'success',
        message: 'Operational settings saved. Restart Sentinel to apply them.',
      })
      pushToast({
        level: 'success',
        title: 'Operational settings saved',
        message: 'The config file is updated; the running process remains unchanged until restart.',
      })
    } catch (error) {
      const conflict = error instanceof ApiError && error.code === 'CONFIG_CONFLICT'
      const message = conflict
        ? 'Settings changed elsewhere. The latest values were loaded and your draft was preserved.'
        : error instanceof Error
          ? error.message
          : 'Failed to save operational settings.'
      setFeedback({ status: 'error', message })
      pushToast({ level: 'error', title: 'Operational settings not saved', message })
    }
  }, [changes, editor, operations, pushToast, settingsQuery])

  const loadError =
    settingsQuery.error instanceof Error
      ? settingsQuery.error.message
      : 'Operational settings could not be loaded.'

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Operations"
        description="Control collection cadence, execution concurrency, and daemon logging from one restart-safe draft."
        icon={<Activity className="size-4" aria-hidden="true" />}
      />

      <SaveFeedback status={feedback.status} message={feedback.message} />

      {settingsQuery.isLoading && operations == null && (
        <div
          aria-label="Loading operational settings"
          className="h-64 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-card"
        />
      )}

      {settingsQuery.isError && operations == null && (
        <div
          role="alert"
          className="grid gap-3 rounded-lg border border-destructive/45 bg-card p-3"
        >
          <p className="text-[11px] text-destructive-foreground">{loadError}</p>
          <Button
            variant="outline"
            className="min-h-11 w-full sm:w-fit"
            onClick={() => void settingsQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      )}

      {settings && operations && editor && (
        <>
          <RestartPendingNotice restart={settings.restart} deployment={settings.deployment} />

          <SettingsGroup
            title="Watchtower"
            description="Project tmux activity into the operational timeline without changing any tmux session."
            icon={<Activity className="size-4" aria-hidden="true" />}
          >
            <SettingsField
              label="Activity projection"
              description="Enable background observation at the next Sentinel start."
              setting={operations.watchtower.enabled}
              htmlFor="settings-watchtower-enabled"
            >
              <BooleanControl
                id="settings-watchtower-enabled"
                value={editor.draft.watchtowerEnabled}
                setting={operations.watchtower.enabled}
                onChange={(value) => updateDraft('watchtowerEnabled', value)}
              />
              <SettingValueSummary setting={operations.watchtower.enabled} />
            </SettingsField>

            <DurationField
              id="settings-watchtower-tick"
              label="Collection interval"
              description="How often Sentinel samples tmux activity. Lower intervals increase collection work."
              value={editor.draft.tickInterval}
              setting={operations.watchtower.tickInterval}
              error={errors.tickInterval ?? ''}
              onChange={(value) => updateDraft('tickInterval', value)}
            />

            <IntegerField
              id="settings-watchtower-lines"
              label="Capture lines"
              description="Maximum pane tail captured per sample. Content remains local."
              value={editor.draft.captureLines}
              setting={operations.watchtower.captureLines}
              error={errors.captureLines ?? ''}
              onChange={(value) => updateDraft('captureLines', value)}
            />

            <DurationField
              id="settings-watchtower-timeout"
              label="Capture timeout"
              description="Maximum time allowed for each pane capture before that sample is skipped."
              value={editor.draft.captureTimeout}
              setting={operations.watchtower.captureTimeout}
              error={errors.captureTimeout ?? ''}
              onChange={(value) => updateDraft('captureTimeout', value)}
            />

            <IntegerField
              id="settings-watchtower-journal"
              label="Journal rows"
              description="Upper bound for retained Watchtower activity rows before pruning."
              value={editor.draft.journalRows}
              setting={operations.watchtower.journalRows}
              error={errors.journalRows ?? ''}
              onChange={(value) => updateDraft('journalRows', value)}
            />
          </SettingsGroup>

          <SettingsGroup
            title="Runbooks"
            description="Bound concurrent procedure execution for predictable host load."
            icon={<BookOpenCheck className="size-4" aria-hidden="true" />}
          >
            <IntegerField
              id="settings-runbooks-concurrency"
              label="Concurrent runbooks"
              description="Applies to new executions after restart; active jobs are never interrupted."
              value={editor.draft.maxConcurrent}
              setting={operations.runbooks.maxConcurrent}
              error={errors.maxConcurrent ?? ''}
              onChange={(value) => updateDraft('maxConcurrent', value)}
            />
          </SettingsGroup>

          <SettingsGroup
            title="Logging"
            description="Choose the daemon verbosity written after the next process start."
            icon={<FileText className="size-4" aria-hidden="true" />}
          >
            <SettingsField
              label="Log level"
              description="The managed service no longer overrides this TOML value."
              setting={operations.log.level}
              htmlFor="settings-log-level"
            >
              <Select
                value={editor.draft.logLevel}
                onValueChange={(value) => updateDraft('logLevel', value)}
                disabled={!operations.log.level.editable || settingsQuery.isSaving}
              >
                <SelectTrigger
                  id="settings-log-level"
                  className="min-h-11 w-full bg-surface-overlay sm:max-w-sm"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {operations.log.level.validation.options.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <ValidationError message={errors.logLevel ?? ''} />
              <EnvironmentOwnership setting={operations.log.level} />
              <SettingValueSummary setting={operations.log.level} />
            </SettingsField>
          </SettingsGroup>

          {dirty && (
            <aside
              aria-label="Unsaved settings"
              className="sticky bottom-2 z-20 grid gap-3 rounded-xl border border-primary/35 bg-surface-overlay/95 p-3 shadow-[0_14px_45px_rgba(0,0,0,0.42)] backdrop-blur sm:grid-cols-[minmax(0,1fr)_auto]"
            >
              <div className="min-w-0">
                <p className="text-[11px] font-medium text-primary-text-bright">
                  {changes.length} unsaved {changes.length === 1 ? 'change' : 'changes'}
                </p>
                <ul className="mt-1.5 grid gap-1 text-[9px] text-muted-foreground">
                  {changes.map((change) => (
                    <li key={change.key} className="min-w-0 break-words">
                      <span className="font-mono text-secondary-foreground">
                        {change.configKey}
                      </span>
                      {' · '}
                      {change.before} → {change.after}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row sm:self-end">
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 w-full sm:w-auto"
                  onClick={discard}
                  disabled={settingsQuery.isSaving}
                >
                  <Undo2 className="size-3.5" aria-hidden="true" />
                  Discard
                </Button>
                <Button
                  type="button"
                  className="min-h-11 w-full sm:w-auto"
                  onClick={() => void save()}
                  disabled={settingsQuery.isSaving}
                >
                  <Save className="size-3.5" aria-hidden="true" />
                  {settingsQuery.isSaving ? 'Saving…' : 'Save changes'}
                </Button>
              </div>
            </aside>
          )}
        </>
      )}

      <AlertDialog open={blocker.status === 'blocked'}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard operational changes?</AlertDialogTitle>
            <AlertDialogDescription>
              This draft has not been written to the config file. Stay here to keep editing, or
              discard it and continue.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              className="min-h-11"
              onClick={() => {
                if (blocker.status === 'blocked') blocker.reset()
              }}
            >
              Stay
            </AlertDialogCancel>
            <AlertDialogAction
              className="min-h-11"
              onClick={() => {
                discard()
                if (blocker.status === 'blocked') blocker.proceed()
              }}
            >
              Discard and leave
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function preserveDraftValue<Key extends OperationsDraftKey>(
  target: OperationsDraft,
  source: OperationsDraft,
  key: Key,
) {
  target[key] = source[key]
}

function SettingsGroup({
  title,
  description,
  icon,
  children,
}: {
  title: string
  description: string
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="grid gap-3">
      <div className="flex items-start gap-2 px-1">
        <span className="mt-0.5 text-primary">{icon}</span>
        <div>
          <h2 className="text-[12px] font-medium">{title}</h2>
          <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="grid gap-3">{children}</div>
    </section>
  )
}

function BooleanControl({
  id,
  value,
  setting,
  onChange,
}: {
  id: string
  value: boolean
  setting: BooleanSetting
  onChange: (value: boolean) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <button
        id={id}
        type="button"
        role="switch"
        aria-label="Activity projection"
        aria-checked={value}
        disabled={!setting.editable}
        onClick={() => onChange(!value)}
        className={cn(
          'relative h-11 w-[4.5rem] rounded-full border transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50',
          value ? 'border-primary/50 bg-primary/25' : 'border-border-subtle bg-surface-overlay',
        )}
      >
        <span
          className={cn(
            'absolute top-1/2 size-5 -translate-y-1/2 rounded-full bg-foreground transition-transform',
            value ? 'translate-x-[2.65rem]' : 'translate-x-2',
          )}
        />
      </button>
      <span className="text-[11px] text-secondary-foreground">
        {value ? 'Enabled after restart' : 'Disabled after restart'}
      </span>
      <EnvironmentOwnership setting={setting} />
    </div>
  )
}

function DurationField({
  id,
  label,
  description,
  value,
  setting,
  error,
  onChange,
}: {
  id: string
  label: string
  description: string
  value: string
  setting: StringSetting
  error: string
  onChange: (value: string) => void
}) {
  const errorID = `${id}-error`
  return (
    <SettingsField label={label} description={description} setting={setting} htmlFor={id}>
      <Input
        id={id}
        value={value}
        aria-invalid={error !== ''}
        aria-describedby={error !== '' ? errorID : `${id}-constraint`}
        disabled={!setting.editable}
        onChange={(event) => onChange(event.target.value)}
        className="min-h-11 bg-surface-overlay font-mono sm:max-w-sm"
      />
      <p id={`${id}-constraint`} className="text-[10px] text-muted-foreground">
        Allowed range: {setting.validation.min} to {setting.validation.max}.
      </p>
      <ValidationError id={errorID} message={error} />
      <EnvironmentOwnership setting={setting} />
      <SettingValueSummary setting={setting} />
    </SettingsField>
  )
}

function IntegerField({
  id,
  label,
  description,
  value,
  setting,
  error,
  onChange,
}: {
  id: string
  label: string
  description: string
  value: string
  setting: IntegerSetting
  error: string
  onChange: (value: string) => void
}) {
  const errorID = `${id}-error`
  return (
    <SettingsField label={label} description={description} setting={setting} htmlFor={id}>
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={setting.validation.min}
        max={setting.validation.max}
        step={setting.validation.step}
        value={value}
        aria-invalid={error !== ''}
        aria-describedby={error !== '' ? errorID : `${id}-constraint`}
        disabled={!setting.editable}
        onChange={(event) => onChange(event.target.value)}
        className="min-h-11 bg-surface-overlay font-mono sm:max-w-sm"
      />
      <p id={`${id}-constraint`} className="text-[10px] text-muted-foreground">
        Allowed range: {setting.validation.min.toLocaleString()} to{' '}
        {setting.validation.max.toLocaleString()}.
      </p>
      <ValidationError id={errorID} message={error} />
      <EnvironmentOwnership setting={setting} />
      <SettingValueSummary setting={setting} />
    </SettingsField>
  )
}

function EnvironmentOwnership({
  setting,
}: {
  setting: BooleanSetting | StringSetting | IntegerSetting
}) {
  if (setting.editable) return null
  return (
    <p className="text-[10px] text-warning-foreground">
      This value is owned by the environment and cannot be changed here.
    </p>
  )
}
