import { useBlocker } from '@tanstack/react-router'
import { Bot, HeartPulse, Plug, Save, Undo2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import HealthReportSettingsPanel from './HealthReportSettingsPanel'
import {
  diffIntegrationsDraft,
  integrationsDraftFromSettings,
  integrationsErrorsFromAPI,
  integrationsPatchFromChanges,
  validateIntegrationsDraft,
} from './integrationsDraft'
import type {
  IntegrationsDraft,
  IntegrationsDraftErrors,
  IntegrationsDraftKey,
} from './integrationsDraft'
import MCPSettingsPanel from './MCPSettingsPanel'
import RestartPendingNotice from './RestartPendingNotice'
import type { SecretIntent } from './SecretSettingControl'
import { SaveFeedback } from './SettingsField'
import SettingsSectionHeader from './SettingsSectionHeader'
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
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useSettings } from '@/hooks/useSettings'
import { ApiError } from '@/hooks/useTmuxApi'

type EditorState = {
  base: IntegrationsDraft
  draft: IntegrationsDraft
}

type Feedback = {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}

const idleFeedback: Feedback = { status: 'idle', message: '' }

export default function IntegrationsSettings() {
  const { hostname } = useMetaContext()
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const integrations = settings?.integrations
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [errors, setErrors] = useState<IntegrationsDraftErrors>({})
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)

  useEffect(() => {
    if (integrations == null) return
    const next = integrationsDraftFromSettings(integrations)
    setEditor((current) => {
      if (current == null) return { base: next, draft: next }
      const pending = diffIntegrationsDraft(current.base, current.draft, integrations)
      if (pending.length === 0) return { base: next, draft: next }
      const preserved = { ...next }
      for (const change of pending) {
        preserveDraftValue(preserved, current.draft, change.key)
      }
      return { base: next, draft: preserved }
    })
  }, [integrations])

  const changes = useMemo(
    () =>
      editor == null || integrations == null
        ? []
        : diffIntegrationsDraft(editor.base, editor.draft, integrations),
    [editor, integrations],
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
    <Key extends keyof IntegrationsDraft>(key: Key, value: IntegrationsDraft[Key]) => {
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
      setFeedback(idleFeedback)
    },
    [],
  )

  const updateTokenIntent = useCallback((intent: SecretIntent) => {
    setEditor((current) => {
      if (current == null) return current
      return {
        ...current,
        draft: {
          ...current.draft,
          tokenIntent: intent,
          tokenValue: intent === 'replace' ? current.draft.tokenValue : '',
          ...(intent === 'clear' ? { mcpEnabled: false } : {}),
        },
      }
    })
    setErrors((current) => ({ ...current, token: undefined }))
    setFeedback(idleFeedback)
  }, [])

  const updateWebhookIntent = useCallback((intent: SecretIntent) => {
    setEditor((current) =>
      current == null
        ? current
        : {
            ...current,
            draft: {
              ...current.draft,
              webhookIntent: intent,
              webhookValue: intent === 'replace' ? current.draft.webhookValue : '',
            },
          },
    )
    setErrors((current) => ({ ...current, webhook: undefined }))
    setFeedback(idleFeedback)
  }, [])

  const discard = useCallback(() => {
    setEditor((current) =>
      current == null ? current : { base: current.base, draft: current.base },
    )
    setErrors({})
    setFeedback({ status: 'success', message: 'Unsaved integration changes discarded.' })
  }, [])

  const save = useCallback(async () => {
    if (editor == null || integrations == null || changes.length === 0) return
    const nextErrors = validateIntegrationsDraft(editor.draft)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      setFeedback({ status: 'error', message: 'Review the highlighted integration values.' })
      return
    }

    const includesSecret = changes.some(
      (change) => change.key === 'token' || change.key === 'webhook',
    )
    const patch = integrationsPatchFromChanges(editor.draft, changes)
    setFeedback({ status: 'saving', message: 'Saving integration settings…' })
    if (includesSecret) {
      setEditor((current) =>
        current == null
          ? current
          : {
              ...current,
              draft: {
                ...current.draft,
                tokenIntent: 'keep',
                tokenValue: '',
                webhookIntent: 'keep',
                webhookValue: '',
              },
            },
      )
    }

    try {
      const snapshot = await settingsQuery.save(patch)
      const next = integrationsDraftFromSettings(snapshot.settings.integrations)
      setEditor({ base: next, draft: next })
      setErrors({})
      setFeedback({
        status: 'success',
        message: snapshot.settings.restart.required
          ? 'Integration settings saved. Review the restart requirement.'
          : 'Integration settings saved and applied.',
      })
      pushToast({
        level: 'success',
        title: 'Integration settings saved',
        message: 'Write-only values were accepted and removed from the browser form.',
      })
    } catch (error) {
      const fieldErrors = error instanceof ApiError ? integrationsErrorsFromAPI(error) : {}
      setErrors(fieldErrors)
      const conflict = error instanceof ApiError && error.code === 'CONFIG_CONFLICT'
      const message = conflict
        ? 'Settings changed elsewhere. Latest values were loaded; re-enter any replacement secret.'
        : includesSecret
          ? 'Integration settings were not saved. Re-enter any replacement secret.'
          : error instanceof Error
            ? error.message
            : 'Failed to save integration settings.'
      setFeedback({ status: 'error', message })
      pushToast({
        level: 'error',
        title: 'Integration settings not saved',
        message: 'Review the form and retry. Replacement secrets were discarded from the browser.',
      })
    }
  }, [changes, editor, integrations, pushToast, settingsQuery])

  if (settingsQuery.isLoading && (integrations == null || editor == null)) {
    return (
      <div className="grid gap-4" aria-label="Loading integration settings">
        <div className="h-12 animate-pulse rounded-lg bg-surface-overlay" />
        <div className="h-48 animate-pulse rounded-lg bg-surface-overlay" />
        <div className="h-48 animate-pulse rounded-lg bg-surface-overlay" />
      </div>
    )
  }

  if (integrations == null || editor == null || settings == null) {
    const loadError =
      settingsQuery.error instanceof Error
        ? settingsQuery.error.message
        : 'Failed to load integration settings'
    return (
      <div role="alert" className="rounded-lg border border-destructive/45 bg-destructive/10 p-4">
        <p className="text-[11px] text-destructive-foreground">{loadError}</p>
        <Button
          type="button"
          variant="outline"
          className="mt-3 min-h-11"
          onClick={() => void settingsQuery.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  const scheduleChanged = editor.base.schedule !== editor.draft.schedule

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Integrations"
        description="Connect trusted tools and scheduled delivery without returning credentials to the browser."
        icon={<Plug className="size-4" aria-hidden="true" />}
      />
      <SaveFeedback status={feedback.status} message={feedback.message} />
      <RestartPendingNotice restart={settings.restart} deployment={settings.deployment} />

      <IntegrationGroup
        title="MCP"
        description="Control trusted agent access and its shared write-only credential."
        icon={<Bot className="size-4" aria-hidden="true" />}
      >
        <MCPSettingsPanel
          hostname={hostname}
          settings={integrations.mcp}
          enabled={editor.draft.mcpEnabled}
          tokenIntent={editor.draft.tokenIntent}
          tokenValue={editor.draft.tokenValue}
          tokenError={errors.token}
          saving={settingsQuery.isSaving}
          onEnabledChange={(enabled) => {
            updateDraft('mcpEnabled', enabled)
            setErrors((current) => ({ ...current, token: undefined }))
          }}
          onTokenIntentChange={updateTokenIntent}
          onTokenValueChange={(value) => {
            updateDraft('tokenValue', value)
            setErrors((current) => ({ ...current, token: undefined }))
          }}
        />
      </IntegrationGroup>

      <IntegrationGroup
        title="Health report"
        description="Schedule concise host health delivery to one write-only webhook."
        icon={<HeartPulse className="size-4" aria-hidden="true" />}
      >
        <HealthReportSettingsPanel
          settings={integrations.healthReport}
          schedule={editor.draft.schedule}
          scheduleChanged={scheduleChanged}
          webhookIntent={editor.draft.webhookIntent}
          webhookValue={editor.draft.webhookValue}
          scheduleError={errors.schedule}
          webhookError={errors.webhook}
          onScheduleChange={(value) => {
            updateDraft('schedule', value)
            setErrors((current) => ({ ...current, schedule: undefined }))
          }}
          onWebhookIntentChange={updateWebhookIntent}
          onWebhookValueChange={(value) => {
            updateDraft('webhookValue', value)
            setErrors((current) => ({ ...current, webhook: undefined }))
          }}
        />
      </IntegrationGroup>

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
                  <span className="font-mono text-secondary-foreground">{change.configKey}</span>
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

      <AlertDialog open={blocker.status === 'blocked'}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard integration changes?</AlertDialogTitle>
            <AlertDialogDescription>
              Unsaved values will be removed. Replacement secrets cannot be recovered from the
              browser.
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

function preserveDraftValue(
  target: IntegrationsDraft,
  source: IntegrationsDraft,
  key: IntegrationsDraftKey,
) {
  switch (key) {
    case 'mcpEnabled':
      target.mcpEnabled = source.mcpEnabled
      break
    case 'token':
      target.tokenIntent = source.tokenIntent
      target.tokenValue = source.tokenValue
      break
    case 'schedule':
      target.schedule = source.schedule
      break
    case 'webhook':
      target.webhookIntent = source.webhookIntent
      target.webhookValue = source.webhookValue
      break
  }
}

function IntegrationGroup({
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
      {children}
    </section>
  )
}
