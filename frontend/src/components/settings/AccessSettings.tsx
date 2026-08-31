import { useBlocker } from '@tanstack/react-router'
import {
  AlertTriangle,
  Check,
  Cookie,
  Copy,
  Globe2,
  KeyRound,
  LifeBuoy,
  Network,
  Plus,
  Router,
  Save,
  ShieldCheck,
  Undo2,
  X,
} from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'

import {
  accessDraftFromSettings,
  accessErrorsFromAPI,
  accessPatchFromChanges,
  candidateReconnectOrigin,
  classifyListenerHost,
  diffAccessDraft,
  validateAccessDraft,
} from './accessDraft'
import type {
  AccessDraft,
  AccessDraftErrors,
  AccessDraftKey,
  AccessDraftChange,
} from './accessDraft'
import RestartPendingNotice from './RestartPendingNotice'
import SecretSettingControl from './SecretSettingControl'
import type { SecretIntent } from './SecretSettingControl'
import {
  EnvironmentOwnership,
  SaveFeedback,
  SettingValueSummary,
  SettingsField,
  ValidationError,
} from './SettingsField'
import SettingsGroup from './SettingsGroup'
import SettingsSectionHeader from './SettingsSectionHeader'
import type { SettingsResponse, StringListSetting } from '@/api/settings'
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
import { Badge } from '@/components/ui/badge'
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
import { writeClipboardText } from '@/lib/clipboardProvider'
import { cn } from '@/lib/utils'

type EditorState = {
  base: AccessDraft
  draft: AccessDraft
}

type Feedback = {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}

const idleFeedback: Feedback = { status: 'idle', message: '' }

export default function AccessSettings() {
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const access = settings?.access
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [editorSource, setEditorSource] = useState<typeof access>(undefined)
  const [errors, setErrors] = useState<AccessDraftErrors>({})
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)
  const [confirmationOpen, setConfirmationOpen] = useState(false)
  const browserEndpoint = useMemo(
    () => ({
      protocol: window.location.protocol || 'http:',
      hostname: window.location.hostname || 'localhost',
    }),
    [],
  )

  if (access != null && access !== editorSource) {
    setEditorSource(access)
    const next = accessDraftFromSettings(access)
    setEditor((current) => {
      if (current == null) return { base: next, draft: next }
      const pending = diffAccessDraft(current.base, current.draft, access)
      if (pending.length === 0) return { base: next, draft: next }
      const preserved = { ...next }
      for (const change of pending) {
        preserveDraftValue(preserved, current.draft, change.key)
      }
      return { base: next, draft: preserved }
    })
  }

  const changes = useMemo(
    () =>
      editor == null || access == null ? [] : diffAccessDraft(editor.base, editor.draft, access),
    [access, editor],
  )
  const reconnectOrigin =
    editor == null ? '' : candidateReconnectOrigin(browserEndpoint, editor.draft)
  const dirty = changes.length > 0
  const blocker = useBlocker({
    shouldBlockFn: useCallback(
      ({ current, next }: { current: { pathname: string }; next: { pathname: string } }) =>
        dirty && current.pathname !== next.pathname,
      [dirty],
    ),
    enableBeforeUnload: dirty,
    withResolver: true,
  })

  const updateDraft = useCallback(
    <Key extends keyof AccessDraft>(key: Key, value: AccessDraft[Key]) => {
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
      setErrors((current) => ({ ...current, [key]: undefined, reconnectOrigin: undefined }))
      setFeedback(idleFeedback)
    },
    [],
  )

  const updateTokenIntent = useCallback((intent: SecretIntent) => {
    setEditor((current) =>
      current == null
        ? current
        : {
            ...current,
            draft: {
              ...current.draft,
              tokenIntent: intent,
              tokenValue: intent === 'replace' ? current.draft.tokenValue : '',
            },
          },
    )
    setErrors((current) => ({ ...current, token: undefined }))
    setFeedback(idleFeedback)
  }, [])

  const discard = useCallback(() => {
    setEditor((current) =>
      current == null ? current : { base: current.base, draft: current.base },
    )
    setErrors({})
    setConfirmationOpen(false)
    setFeedback({ status: 'success', message: 'Unsaved access changes discarded.' })
  }, [])

  const reviewSave = useCallback(() => {
    if (editor == null || access == null || changes.length === 0) return
    const nextErrors = validateAccessDraft(editor.draft, access, browserEndpoint)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      setFeedback({
        status: 'error',
        message: 'Review the highlighted access controls before continuing.',
      })
      return
    }
    setConfirmationOpen(true)
  }, [access, browserEndpoint, changes.length, editor])

  const confirmSave = useCallback(async () => {
    if (editor == null || access == null || changes.length === 0 || reconnectOrigin === '') return
    const submittedChanges = [...changes]
    const patch = accessPatchFromChanges(editor.draft, submittedChanges, reconnectOrigin)
    const includesToken = submittedChanges.some((change) => change.key === 'token')
    setConfirmationOpen(false)
    setFeedback({ status: 'saving', message: 'Saving guarded access settings…' })
    if (includesToken) {
      setEditor((current) =>
        current == null
          ? current
          : {
              ...current,
              draft: {
                ...current.draft,
                tokenIntent: 'keep',
                tokenValue: '',
              },
            },
      )
    }

    try {
      const snapshot = await settingsQuery.save(patch)
      const next = accessDraftFromSettings(snapshot.settings.access)
      setEditor({ base: next, draft: next })
      setErrors({})
      setFeedback({
        status: 'success',
        message: 'Access settings saved. Review recovery, then restart Sentinel from the notice.',
      })
      pushToast({
        level: 'success',
        title: 'Access settings saved',
        message: includesToken
          ? 'The replacement token was removed from the browser. Restart will require reauthentication.'
          : 'The candidate passed validation and bind preflight before it was written.',
      })
    } catch (error) {
      setErrors(error instanceof ApiError ? accessErrorsFromAPI(error) : {})
      const conflict = error instanceof ApiError && error.code === 'CONFIG_CONFLICT'
      const message = conflict
        ? 'Settings changed elsewhere. Latest safe metadata was loaded; re-enter any token replacement.'
        : includesToken
          ? 'Access settings were not saved. Re-enter the replacement token before retrying.'
          : error instanceof Error
            ? error.message
            : 'Access settings were not saved.'
      setFeedback({ status: 'error', message })
      pushToast({
        level: 'error',
        title: 'Access settings not saved',
        message: includesToken
          ? 'The submitted token was discarded from this browser.'
          : 'The config file was left unchanged.',
      })
    }
  }, [access, changes, editor, pushToast, reconnectOrigin, settingsQuery])

  if (settingsQuery.isLoading && (access == null || editor == null)) {
    return (
      <div className="grid gap-4" aria-label="Loading access settings">
        <div className="h-12 animate-pulse rounded-lg bg-surface-overlay" />
        <div className="h-56 animate-pulse rounded-lg bg-surface-overlay" />
        <div className="h-56 animate-pulse rounded-lg bg-surface-overlay" />
      </div>
    )
  }

  if (settings == null || access == null || editor == null) {
    const message =
      settingsQuery.error instanceof Error
        ? settingsQuery.error.message
        : 'Access settings could not be loaded.'
    return (
      <div role="alert" className="rounded-lg border border-destructive/45 bg-destructive/10 p-4">
        <p className="text-[11px] text-destructive-foreground">{message}</p>
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

  const classification = classifyListenerHost(editor.draft.host)
  const tokenWillRotate = changes.some((change) => change.key === 'token')
  const remote = classification !== 'loopback'
  const insecureRemote = remote && browserEndpoint.protocol !== 'https:'

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Access"
        description="Change the listener, shared credential, origin boundary, proxies, and cookies as one guarded candidate."
        icon={<ShieldCheck className="size-4" aria-hidden="true" />}
      />
      <SaveFeedback status={feedback.status} message={feedback.message} />
      <RestartPendingNotice
        revision={settings.revision}
        restart={settings.restart}
        deployment={settings.deployment}
        reconnectOrigin={reconnectOrigin}
        onRestartComplete={() => setFeedback(idleFeedback)}
      />

      <SettingsGroup
        title="Listener"
        description="The candidate is bind-checked before persistence. Wildcard listeners reconnect through this browser's hostname."
        icon={<Network className="size-4" aria-hidden="true" />}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <SettingsField
            label="Host"
            description="Use loopback for host-only access, a wildcard for all interfaces, or one specific address."
            setting={access.listener.host}
            htmlFor="settings-access-host"
          >
            <Input
              id="settings-access-host"
              value={editor.draft.host}
              disabled={!access.listener.host.editable}
              spellCheck={false}
              autoCapitalize="none"
              className="min-h-11 bg-surface-overlay font-mono"
              aria-invalid={errors.host ? true : undefined}
              onChange={(event) => updateDraft('host', event.target.value)}
            />
            <div className="flex flex-wrap items-center gap-2">
              <CandidateBadge classification={classification} />
              <span className="text-[10px] text-muted-foreground">
                Current runtime · <span className="font-mono">{access.listener.address}</span>
              </span>
            </div>
            <ValidationError message={errors.host ?? ''} />
            <EnvironmentOwnership setting={access.listener.host} />
            <SettingValueSummary setting={access.listener.host} />
          </SettingsField>

          <SettingsField
            label="Port"
            description="A changed port must be available to this process before Sentinel writes it."
            setting={access.listener.port}
            htmlFor="settings-access-port"
          >
            <Input
              id="settings-access-port"
              type="number"
              inputMode="numeric"
              min={access.listener.port.validation.min}
              max={access.listener.port.validation.max}
              step={access.listener.port.validation.step}
              value={editor.draft.port}
              disabled={!access.listener.port.editable}
              className="min-h-11 bg-surface-overlay font-mono"
              aria-invalid={errors.port ? true : undefined}
              onChange={(event) => updateDraft('port', event.target.value)}
            />
            <ValidationError message={errors.port ?? ''} />
            <EnvironmentOwnership setting={access.listener.port} />
            <SettingValueSummary setting={access.listener.port} />
          </SettingsField>
        </div>

        <div
          className={cn(
            'rounded-lg border p-3',
            insecureRemote
              ? 'border-warning/45 bg-warning/10 text-warning-foreground'
              : 'border-primary/25 bg-primary/10 text-primary-text',
          )}
        >
          <p className="text-[10px] uppercase tracking-[0.08em]">Reconnect target</p>
          <p className="mt-1 break-all font-mono text-[11px]">
            {reconnectOrigin || 'Unavailable until host and port are valid'}
          </p>
          {insecureRemote && (
            <p className="mt-2 text-[10px] leading-relaxed">
              This candidate is remotely reachable over plain HTTP. Prefer TLS at a trusted reverse
              proxy or private tunnel.
            </p>
          )}
          <ValidationError message={errors.reconnectOrigin ?? ''} />
        </div>
      </SettingsGroup>

      <SettingsGroup
        title="Authentication"
        description="One shared operator token protects the SPA, API, WebSockets, and MCP."
        icon={<KeyRound className="size-4" aria-hidden="true" />}
      >
        <SettingsField
          label="Shared token"
          description="Keep, replace once, or clear only when the complete candidate remains safe."
          setting={access.authentication.token}
        >
          <SecretSettingControl
            id="settings-access-token"
            label="Shared token"
            setting={access.authentication.token}
            intent={editor.draft.tokenIntent}
            value={editor.draft.tokenValue}
            error={errors.token}
            placeholder="new shared token"
            onIntentChange={updateTokenIntent}
            onValueChange={(value) => updateDraft('tokenValue', value)}
          />
          <p className="text-[10px] text-muted-foreground">
            Running process ·{' '}
            {access.authentication.runtimeTokenConfigured
              ? 'authentication enabled'
              : 'authentication disabled'}
          </p>
        </SettingsField>
      </SettingsGroup>

      <SettingsGroup
        title="Origins"
        description="Explicit browser origins accepted in addition to the listener's own same-origin requests."
        icon={<Globe2 className="size-4" aria-hidden="true" />}
      >
        <SettingsField
          label="Allowed origins"
          description="Remote exposure requires at least one canonical HTTP(S) origin."
          setting={access.origins.allowed}
          htmlFor="settings-allowed-origins-new"
        >
          <StringListEditor
            id="settings-allowed-origins"
            label="origin"
            placeholder="https://sentinel.example.com"
            values={editor.draft.allowedOrigins}
            disabled={!access.origins.allowed.editable}
            onChange={(values) => updateDraft('allowedOrigins', values)}
          />
          <ValidationError message={errors.allowedOrigins ?? ''} />
          <EnvironmentOwnership setting={access.origins.allowed} />
          <ListSettingSummary setting={access.origins.allowed} />
        </SettingsField>
      </SettingsGroup>

      <SettingsGroup
        title="Trusted proxies"
        description="Only direct peers in this list may supply HTTPS forwarding metadata; loopback peers are trusted automatically."
        icon={<Router className="size-4" aria-hidden="true" />}
      >
        <SettingsField
          label="Proxy IPs and CIDRs"
          description="Use exact IP addresses or network ranges. Do not enter proxy hostnames."
          setting={access.proxies.trusted}
          htmlFor="settings-trusted-proxies-new"
        >
          <StringListEditor
            id="settings-trusted-proxies"
            label="proxy"
            placeholder="10.0.0.0/8"
            values={editor.draft.trustedProxies}
            disabled={!access.proxies.trusted.editable}
            onChange={(values) => updateDraft('trustedProxies', values)}
          />
          <ValidationError message={errors.trustedProxies ?? ''} />
          <EnvironmentOwnership setting={access.proxies.trusted} />
          <ListSettingSummary setting={access.proxies.trusted} />
        </SettingsField>
      </SettingsGroup>

      <SettingsGroup
        title="Cookies"
        description="Control when the HttpOnly authentication cookie is marked Secure."
        icon={<Cookie className="size-4" aria-hidden="true" />}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <SettingsField
            label="Secure policy"
            description="Auto follows the trusted request scheme. Always requires an HTTPS reconnect target."
            setting={access.cookies.secure}
            htmlFor="settings-cookie-secure"
          >
            <Select
              value={editor.draft.cookieSecure}
              disabled={!access.cookies.secure.editable}
              onValueChange={(value) => updateDraft('cookieSecure', value)}
            >
              <SelectTrigger id="settings-cookie-secure" className="min-h-11 bg-surface-overlay">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {access.cookies.secure.validation.options.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <ValidationError message={errors.cookieSecure ?? ''} />
            <EnvironmentOwnership setting={access.cookies.secure} />
            <SettingValueSummary setting={access.cookies.secure} />
          </SettingsField>

          <SettingsField
            label="Allow insecure cookie"
            description="Explicit exception required for remote token authentication with never-secure cookies."
            setting={access.cookies.allowInsecure}
            htmlFor="settings-allow-insecure-cookie"
          >
            <button
              id="settings-allow-insecure-cookie"
              type="button"
              role="switch"
              aria-checked={editor.draft.allowInsecureCookie}
              disabled={!access.cookies.allowInsecure.editable}
              onClick={() => updateDraft('allowInsecureCookie', !editor.draft.allowInsecureCookie)}
              className={cn(
                'flex min-h-11 w-full items-center justify-between gap-3 rounded-md border px-3 text-left text-[11px] transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-55',
                editor.draft.allowInsecureCookie
                  ? 'border-warning/45 bg-warning/10 text-warning-foreground'
                  : 'border-border-subtle bg-surface-overlay text-secondary-foreground',
              )}
            >
              <span>
                {editor.draft.allowInsecureCookie ? 'Exception enabled' : 'Exception disabled'}
              </span>
              <span
                aria-hidden="true"
                className={cn(
                  'relative h-5 w-9 rounded-full transition-colors',
                  editor.draft.allowInsecureCookie ? 'bg-warning' : 'bg-border',
                )}
              >
                <span
                  className={cn(
                    'absolute top-0.5 size-4 rounded-full bg-background transition-transform',
                    editor.draft.allowInsecureCookie ? 'translate-x-[18px]' : 'translate-x-0.5',
                  )}
                />
              </span>
            </button>
            <ValidationError message={errors.allowInsecureCookie ?? ''} />
            <EnvironmentOwnership setting={access.cookies.allowInsecure} />
            <SettingValueSummary setting={access.cookies.allowInsecure} />
          </SettingsField>
        </div>
      </SettingsGroup>

      <RecoveryPanel recovery={access.recovery} />

      {dirty && (
        <aside
          aria-label="Unsaved settings"
          className="sticky bottom-2 z-20 grid gap-3 rounded-xl border border-warning/40 bg-surface-overlay/95 p-3 shadow-[0_14px_45px_rgba(0,0,0,0.42)] backdrop-blur sm:grid-cols-[minmax(0,1fr)_auto]"
        >
          <div className="min-w-0">
            <p className="text-[11px] font-medium text-warning-foreground">
              {changes.length} guarded {changes.length === 1 ? 'change' : 'changes'}
            </p>
            <p className="mt-1 break-all font-mono text-[9px] text-muted-foreground">
              Reconnect · {reconnectOrigin || 'invalid candidate'}
            </p>
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
              className="min-h-11 w-full"
              onClick={reviewSave}
              disabled={settingsQuery.isSaving}
            >
              <Save className="size-3.5" aria-hidden="true" />
              Review and save
            </Button>
          </div>
        </aside>
      )}

      <AccessConfirmation
        open={confirmationOpen}
        changes={changes}
        reconnectOrigin={reconnectOrigin}
        tokenWillRotate={tokenWillRotate}
        restart={settings.restart}
        onCancel={() => setConfirmationOpen(false)}
        onConfirm={() => void confirmSave()}
      />

      <AlertDialog open={blocker.status === 'blocked'}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard access changes?</AlertDialogTitle>
            <AlertDialogDescription>
              The guarded candidate has not been written. Any replacement token will be removed from
              this browser.
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

function AccessConfirmation({
  open,
  changes,
  reconnectOrigin,
  tokenWillRotate,
  restart,
  onCancel,
  onConfirm,
}: {
  open: boolean
  changes: Array<AccessDraftChange>
  reconnectOrigin: string
  tokenWillRotate: boolean
  restart: SettingsResponse['restart']
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Save this access boundary?</AlertDialogTitle>
          <AlertDialogDescription>
            Sentinel will validate the complete candidate and test a changed listener before
            writing. The running endpoint does not change until Sentinel restarts.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid max-h-[45vh] gap-3 overflow-y-auto text-[10px]">
          <div className="rounded-md border border-border-subtle bg-surface-overlay p-3">
            <p className="uppercase tracking-[0.08em] text-muted-foreground">
              Reconnect after restart
            </p>
            <p className="mt-1 break-all font-mono text-secondary-foreground">{reconnectOrigin}</p>
          </div>
          <ul className="grid gap-1.5 rounded-md border border-border-subtle bg-background/40 p-3">
            {changes.map((change) => (
              <li key={change.key} className="break-words">
                <span className="font-mono text-secondary-foreground">{change.configKey}</span>
                {' · '}
                {change.before} → {change.after}
              </li>
            ))}
          </ul>
          {tokenWillRotate && (
            <p className="rounded-md border border-warning/45 bg-warning/10 p-3 leading-relaxed text-warning-foreground">
              The current browser session will stop authenticating after restart. The new token is
              not retained, so be ready to enter it again in the authentication gate.
            </p>
          )}
          <p className="rounded-md border border-primary/25 bg-primary/10 p-3 leading-relaxed text-primary-text">
            After saving, use the pending restart notice or run the recovery command yourself.
            {restart.command ? ` Command: ${restart.command}` : ''}
          </p>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11" onClick={onCancel}>
            Keep reviewing
          </AlertDialogCancel>
          <AlertDialogAction className="min-h-11" onClick={onConfirm}>
            Save guarded candidate
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function StringListEditor({
  id,
  label,
  placeholder,
  values,
  disabled,
  onChange,
}: {
  id: string
  label: string
  placeholder: string
  values: Array<string>
  disabled: boolean
  onChange: (values: Array<string>) => void
}) {
  const [candidate, setCandidate] = useState('')
  const add = () => {
    const value = candidate.trim()
    if (value === '' || values.includes(value)) return
    onChange([...values, value])
    setCandidate('')
  }
  return (
    <div className="grid gap-2">
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
        <Input
          id={`${id}-new`}
          value={candidate}
          disabled={disabled}
          placeholder={placeholder}
          spellCheck={false}
          autoCapitalize="none"
          className="min-h-11 bg-surface-overlay font-mono"
          aria-label={`New ${label}`}
          onChange={(event) => setCandidate(event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== 'Enter') return
            event.preventDefault()
            add()
          }}
        />
        <Button
          type="button"
          variant="outline"
          className="min-h-11 w-full sm:w-auto"
          disabled={disabled || candidate.trim() === '' || values.includes(candidate.trim())}
          onClick={add}
        >
          <Plus className="size-3.5" aria-hidden="true" />
          Add
        </Button>
      </div>
      <div className="flex min-h-8 flex-wrap gap-2" aria-label={`Configured ${label}s`}>
        {values.length === 0 && (
          <span className="text-[10px] text-muted-foreground">No explicit {label}s</span>
        )}
        {values.map((value) => (
          <span
            key={value}
            className="inline-flex min-h-9 max-w-full items-center gap-1 rounded-md border border-border-subtle bg-surface-overlay pl-2 font-mono text-[10px] text-secondary-foreground"
          >
            <span className="min-w-0 break-all">{value}</span>
            <button
              type="button"
              className="grid min-h-9 min-w-9 place-items-center rounded-r-md text-muted-foreground hover:bg-destructive/10 hover:text-destructive-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-50"
              aria-label={`Remove ${value}`}
              disabled={disabled}
              onClick={() => onChange(values.filter((candidateValue) => candidateValue !== value))}
            >
              <X className="size-3.5" aria-hidden="true" />
            </button>
          </span>
        ))}
      </div>
    </div>
  )
}

function RecoveryPanel({ recovery }: { recovery: SettingsResponse['access']['recovery'] }) {
  return (
    <SettingsGroup
      title="Manual recovery"
      description="Sentinel never rolls back automatically. Keep these commands reachable before changing access or restarting the managed service."
      icon={<LifeBuoy className="size-4" aria-hidden="true" />}
    >
      <div className="grid gap-3 rounded-lg border border-warning/35 bg-warning/10 p-3 sm:p-4">
        <p className="text-[10px] leading-relaxed text-warning-foreground">
          {recovery.instruction}
        </p>
        <RecoveryCommand label="1. Restore backup" command={recovery.restoreCommand} />
        <RecoveryCommand label="2. Validate effective config" command={recovery.validateCommand} />
        {recovery.restartCommand && (
          <RecoveryCommand label="3. Restart deployment" command={recovery.restartCommand} />
        )}
        <dl className="grid gap-2 text-[9px] text-muted-foreground sm:grid-cols-2">
          <div className="min-w-0">
            <dt>Config</dt>
            <dd className="mt-0.5 break-all font-mono text-secondary-foreground">
              {recovery.configPath}
            </dd>
          </div>
          <div className="min-w-0">
            <dt>Backup</dt>
            <dd className="mt-0.5 break-all font-mono text-secondary-foreground">
              {recovery.backupPath}
            </dd>
          </div>
        </dl>
      </div>
    </SettingsGroup>
  )
}

function RecoveryCommand({ label, command }: { label: string; command: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="grid gap-1.5">
      <p className="text-[9px] uppercase tracking-[0.08em] text-warning-foreground">{label}</p>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
        <code className="block overflow-x-auto rounded border border-warning/25 bg-background/45 p-2.5 text-[10px] whitespace-nowrap text-secondary-foreground">
          {command}
        </code>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 w-full border-warning/30 bg-background/35 sm:w-auto"
          onClick={() => {
            setCopied(false)
            void writeClipboardText(command).then(setCopied)
          }}
        >
          {copied ? (
            <Check className="size-3.5" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
    </div>
  )
}

function CandidateBadge({
  classification,
}: {
  classification: 'loopback' | 'wildcard' | 'specific'
}) {
  const labels = {
    loopback: 'Loopback only',
    wildcard: 'All interfaces',
    specific: 'Specific address',
  }
  return (
    <Badge
      variant="outline"
      className={cn(
        'text-[9px]',
        classification === 'loopback'
          ? 'border-ok/35 bg-ok/10 text-ok-foreground'
          : 'border-warning/40 bg-warning/10 text-warning-foreground',
      )}
    >
      {classification !== 'loopback' && (
        <AlertTriangle className="mr-1 size-3" aria-hidden="true" />
      )}
      {labels[classification]}
    </Badge>
  )
}

function ListSettingSummary({ setting }: { setting: StringListSetting }) {
  const persisted =
    setting.persistedValue === undefined
      ? 'Inherited'
      : setting.persistedValue.length === 0
        ? 'Empty'
        : setting.persistedValue.join(', ')
  return (
    <dl className="grid gap-1.5 rounded-md border border-border-subtle bg-background/45 p-2.5 text-[10px] sm:grid-cols-2">
      <div className="min-w-0">
        <dt className="uppercase tracking-[0.08em] text-muted-foreground">Effective</dt>
        <dd className="mt-0.5 break-words font-mono text-secondary-foreground">
          {setting.effectiveValue.length === 0 ? 'Empty' : setting.effectiveValue.join(', ')}
        </dd>
      </div>
      <div className="min-w-0">
        <dt className="uppercase tracking-[0.08em] text-muted-foreground">Persisted</dt>
        <dd className="mt-0.5 break-words font-mono text-secondary-foreground">{persisted}</dd>
      </div>
    </dl>
  )
}

function preserveDraftValue(target: AccessDraft, source: AccessDraft, key: AccessDraftKey) {
  switch (key) {
    case 'host':
      target.host = source.host
      break
    case 'port':
      target.port = source.port
      break
    case 'token':
      target.tokenIntent = source.tokenIntent
      target.tokenValue = source.tokenValue
      break
    case 'allowedOrigins':
      target.allowedOrigins = source.allowedOrigins
      break
    case 'trustedProxies':
      target.trustedProxies = source.trustedProxies
      break
    case 'cookieSecure':
      target.cookieSecure = source.cookieSecure
      break
    case 'allowInsecureCookie':
      target.allowInsecureCookie = source.allowInsecureCookie
      break
  }
}
