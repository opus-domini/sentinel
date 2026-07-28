import { useBlocker } from '@tanstack/react-router'
import { Check, Search, Save, ShieldAlert, Undo2, UserRoundCog, UsersRound } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import {
  accountsDraftFromSettings,
  accountsErrorsFromAPI,
  accountsPatchFromChanges,
  diffAccountsDraft,
  validateAccountsDraft,
} from './accountsDraft'
import type { AccountsDraft, AccountsDraftErrors, AccountsDraftKey } from './accountsDraft'
import RestartPendingNotice from './RestartPendingNotice'
import { EnvironmentOwnership, SaveFeedback, SettingsField, ValidationError } from './SettingsField'
import SettingsSectionHeader from './SettingsSectionHeader'
import SettingsSwitch from './SettingsSwitch'
import type { SettingsResponse } from '@/api/settings'
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
import { cn } from '@/lib/utils'

type EditorState = {
  base: AccountsDraft
  draft: AccountsDraft
}

type Feedback = {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}

const idleFeedback: Feedback = { status: 'idle', message: '' }

export default function AccountsSettings() {
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const accounts = settings?.accounts
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [errors, setErrors] = useState<AccountsDraftErrors>({})
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)
  const [search, setSearch] = useState('')
  const [rootConfirmationOpen, setRootConfirmationOpen] = useState(false)

  useEffect(() => {
    if (accounts == null) return
    const next = accountsDraftFromSettings(accounts)
    setEditor((current) => {
      if (current == null) return { base: next, draft: next }
      const pending = diffAccountsDraft(current.base, current.draft)
      if (pending.length === 0) return { base: next, draft: next }
      const preserved = { ...next }
      for (const change of pending) {
        preserveDraftValue(preserved, current.draft, change.key)
      }
      return { base: next, draft: preserved }
    })
  }, [accounts])

  const changes = useMemo(
    () => (editor == null ? [] : diffAccountsDraft(editor.base, editor.draft)),
    [editor],
  )
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
    <Key extends AccountsDraftKey>(key: Key, value: AccountsDraft[Key]) => {
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

  const setRootTargeting = useCallback(
    (enabled: boolean) => {
      if (editor == null || accounts == null) return
      const allowedUsers = editor.draft.allowedUsers
      const nextAllowed =
        accounts.allowedUsers.editable && allowedUsers.length > 0
          ? enabled
            ? [...new Set([...allowedUsers, 'root'])].sort()
            : allowedUsers.filter((name) => name !== 'root')
          : allowedUsers
      setEditor({
        ...editor,
        draft: {
          ...editor.draft,
          allowRootTarget: enabled,
          allowedUsers: nextAllowed,
        },
      })
      setErrors((current) => ({
        ...current,
        allowRootTarget: undefined,
        allowedUsers: undefined,
      }))
      setFeedback(idleFeedback)
    },
    [accounts, editor],
  )

  const toggleAccount = useCallback(
    (name: string) => {
      if (editor == null || accounts == null || !accounts.allowedUsers.editable) return
      if (name === 'root' && !editor.draft.allowRootTarget) return
      const selectable = accounts.users
        .filter((account) => account.name !== 'root' || editor.draft.allowRootTarget)
        .map((account) => account.name)
      const selected =
        editor.draft.allowedUsers.length === 0 ? selectable : editor.draft.allowedUsers
      const next = selected.includes(name)
        ? selected.filter((candidate) => candidate !== name)
        : [...selected, name]
      const normalized =
        next.length === selectable.length &&
        selectable.every((candidate) => next.includes(candidate))
          ? []
          : [...new Set(next)].sort()
      updateDraft('allowedUsers', normalized)
    },
    [accounts, editor, updateDraft],
  )

  const discard = useCallback(() => {
    setEditor((current) =>
      current == null ? current : { base: current.base, draft: current.base },
    )
    setErrors({})
    setFeedback({ status: 'success', message: 'Unsaved account changes discarded.' })
  }, [])

  const save = useCallback(async () => {
    if (editor == null || accounts == null || changes.length === 0) return
    const nextErrors = validateAccountsDraft(editor.draft, accounts)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      setFeedback({ status: 'error', message: 'Review the highlighted account controls.' })
      return
    }
    setFeedback({ status: 'saving', message: 'Saving account settings…' })
    try {
      const snapshot = await settingsQuery.save(accountsPatchFromChanges(editor.draft, changes))
      const next = accountsDraftFromSettings(snapshot.settings.accounts)
      setEditor({ base: next, draft: next })
      setErrors({})
      setFeedback({
        status: 'success',
        message: 'Account settings saved. Restart Sentinel to apply them.',
      })
      pushToast({
        level: 'success',
        title: 'Account settings saved',
        message: 'No OS account was changed; the new targeting policy applies after restart.',
      })
    } catch (error) {
      setErrors(error instanceof ApiError ? accountsErrorsFromAPI(error) : {})
      const conflict = error instanceof ApiError && error.code === 'CONFIG_CONFLICT'
      const message = conflict
        ? 'Settings changed elsewhere. The latest values were loaded and your draft was preserved.'
        : error instanceof Error
          ? error.message
          : 'Failed to save account settings.'
      setFeedback({ status: 'error', message })
      pushToast({ level: 'error', title: 'Account settings not saved', message })
    }
  }, [accounts, changes, editor, pushToast, settingsQuery])

  if (settingsQuery.isLoading && (accounts == null || editor == null)) {
    return (
      <div className="grid gap-4" aria-label="Loading account settings">
        <div className="h-12 animate-pulse rounded-lg bg-surface-overlay" />
        <div className="h-56 animate-pulse rounded-lg bg-surface-overlay" />
      </div>
    )
  }

  if (settings == null || accounts == null || editor == null) {
    const message =
      settingsQuery.error instanceof Error
        ? settingsQuery.error.message
        : 'Account settings could not be loaded.'
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

  const filteredUsers = accounts.users.filter((account) =>
    account.name.toLowerCase().includes(search.trim().toLowerCase()),
  )
  const wildcardAllowlist = editor.draft.allowedUsers.length === 0

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Accounts"
        description="Choose which existing OS accounts Sentinel may target. This workspace never creates, removes, or modifies accounts."
        icon={<UsersRound className="size-4" aria-hidden="true" />}
      />
      <SaveFeedback status={feedback.status} message={feedback.message} />
      <RestartPendingNotice
        revision={settings.revision}
        restart={settings.restart}
        deployment={settings.deployment}
        onRestartComplete={() => setFeedback(idleFeedback)}
      />

      <SettingsGroup
        title="Process boundary"
        description="Read-only identity and account inventory loaded when this Sentinel process started."
        icon={<UserRoundCog className="size-4" aria-hidden="true" />}
      >
        <div className="grid gap-3 rounded-lg border border-border-subtle bg-card p-3 sm:grid-cols-3 sm:p-4">
          <IdentityDatum label="Process user" value={accounts.processUser || 'Unavailable'}>
            {accounts.processIsRoot && <StatusBadge tone="warning">Root process</StatusBadge>}
          </IdentityDatum>
          <IdentityDatum
            label="Detected accounts"
            value={accounts.inventoryAvailable ? String(accounts.users.length) : 'Unavailable'}
          >
            <StatusBadge tone={accounts.inventoryAvailable ? 'ok' : 'warning'}>
              Read only
            </StatusBadge>
          </IdentityDatum>
          <IdentityDatum
            label="Desired policy"
            value={
              wildcardAllowlist ? 'All detected' : `${editor.draft.allowedUsers.length} explicit`
            }
          >
            <StatusBadge tone="primary">After restart</StatusBadge>
          </IdentityDatum>
        </div>
      </SettingsGroup>

      <SettingsGroup
        title="Target allowlist"
        description="An empty allowlist means every detected account is eligible, with root still controlled separately."
        icon={<UsersRound className="size-4" aria-hidden="true" />}
      >
        <SettingsField
          label="Allowed OS accounts"
          description="Search the startup inventory and choose a closed set. Selecting or clearing accounts never changes the OS."
          setting={accounts.allowedUsers}
          htmlFor="settings-account-search"
        >
          <div className="relative">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              id="settings-account-search"
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search detected accounts"
              className="min-h-11 bg-surface-overlay pl-9"
            />
          </div>
          <div className="flex flex-wrap items-center gap-2 text-[10px]">
            <StatusBadge tone={wildcardAllowlist ? 'ok' : 'primary'}>
              {wildcardAllowlist ? 'All detected accounts' : 'Explicit allowlist'}
            </StatusBadge>
            <span className="text-muted-foreground">
              Root is {editor.draft.allowRootTarget ? 'eligible' : 'blocked'} by its independent
              safety gate.
            </span>
          </div>
          {!accounts.inventoryAvailable && (
            <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-[10px] text-warning-foreground">
              No OS inventory was available at startup. Account targeting is unavailable until
              Sentinel can load it on restart.
            </p>
          )}
          <div className="grid gap-2 sm:grid-cols-2" aria-label="Detected OS accounts">
            {filteredUsers.map((account) => {
              const allowed =
                (wildcardAllowlist || editor.draft.allowedUsers.includes(account.name)) &&
                (account.name !== 'root' || editor.draft.allowRootTarget)
              const disabled =
                !accounts.allowedUsers.editable ||
                (account.name === 'root' && !editor.draft.allowRootTarget)
              return (
                <button
                  key={account.name}
                  type="button"
                  role="checkbox"
                  aria-checked={allowed}
                  aria-label={`${allowed ? 'Disallow' : 'Allow'} ${account.name}`}
                  disabled={disabled}
                  onClick={() => toggleAccount(account.name)}
                  className={cn(
                    'flex min-h-11 min-w-0 items-center gap-3 rounded-lg border p-3 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-55',
                    allowed
                      ? 'border-primary/30 bg-primary/10'
                      : 'border-border-subtle bg-surface-overlay',
                  )}
                >
                  <span
                    className={cn(
                      'grid size-5 shrink-0 place-items-center rounded border',
                      allowed
                        ? 'border-primary bg-primary text-primary-foreground'
                        : 'border-border bg-background',
                    )}
                    aria-hidden="true"
                  >
                    {allowed && <Check className="size-3.5" />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-mono text-[11px] text-foreground">
                      {account.name}
                    </span>
                    <span className="mt-1 flex flex-wrap gap-1">
                      {account.processUser && (
                        <StatusBadge tone="primary">Process user</StatusBadge>
                      )}
                      {account.root && <StatusBadge tone="warning">Root</StatusBadge>}
                      <StatusBadge tone={allowed ? 'ok' : 'muted'}>
                        {allowed ? 'Allowed' : 'Blocked'}
                      </StatusBadge>
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
          {filteredUsers.length === 0 && (
            <p className="rounded-md border border-border-subtle bg-surface-overlay p-3 text-[10px] text-muted-foreground">
              No detected account matches “{search}”.
            </p>
          )}
          <ValidationError message={errors.allowedUsers ?? ''} />
          <EnvironmentOwnership setting={accounts.allowedUsers} />
          <ListValueSummary setting={accounts.allowedUsers} />
        </SettingsField>
      </SettingsGroup>

      <SettingsGroup
        title="Privilege boundary"
        description="Root access is a separate explicit risk decision; Sentinel cannot grant host permissions."
        icon={<ShieldAlert className="size-4" aria-hidden="true" />}
      >
        <SettingsField
          label="Allow root targeting"
          description="Root remains blocked even when the allowlist is empty. Enabling it requires confirmation."
          setting={accounts.allowRootTarget}
          htmlFor="settings-allow-root"
        >
          <div className="flex flex-wrap items-center gap-3">
            <SettingsSwitch
              id="settings-allow-root"
              label="Allow root targeting"
              checked={editor.draft.allowRootTarget}
              disabled={!accounts.allowRootTarget.editable}
              tone="warning"
              onCheckedChange={(checked) => {
                if (!checked) {
                  setRootTargeting(false)
                } else {
                  setRootConfirmationOpen(true)
                }
              }}
            />
            <span className="text-[11px] text-secondary-foreground">
              {editor.draft.allowRootTarget ? 'Root eligible after restart' : 'Root blocked'}
            </span>
          </div>
          <ValidationError message={errors.allowRootTarget ?? ''} />
          <EnvironmentOwnership setting={accounts.allowRootTarget} />
        </SettingsField>

        <SettingsField
          label="User switch method"
          description="The method is closed to sudo or systemd-run. Availability only detects executables, not host policy."
          setting={accounts.userSwitchMethod}
          htmlFor="settings-switch-method"
        >
          <Select
            value={editor.draft.userSwitchMethod}
            onValueChange={(value) => updateDraft('userSwitchMethod', value)}
            disabled={!accounts.userSwitchMethod.editable || settingsQuery.isSaving}
          >
            <SelectTrigger
              id="settings-switch-method"
              className="min-h-11 w-full bg-surface-overlay sm:max-w-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {accounts.userSwitchMethod.validation.options.map((option) => {
                const capability = accounts.methodCapabilities.find(
                  (candidate) => candidate.value === option.value,
                )
                return (
                  <SelectItem
                    key={option.value}
                    value={option.value}
                    disabled={
                      capability?.available === false &&
                      option.value !== editor.draft.userSwitchMethod
                    }
                  >
                    {option.label}
                    {capability?.available === false ? ' · unavailable' : ''}
                  </SelectItem>
                )
              })}
            </SelectContent>
          </Select>
          <ValidationError message={errors.userSwitchMethod ?? ''} />
          <EnvironmentOwnership setting={accounts.userSwitchMethod} />
          <div className="grid gap-2 sm:grid-cols-2">
            {accounts.methodCapabilities.map((capability) => (
              <div
                key={capability.value}
                className="rounded-md border border-border-subtle bg-background/45 p-2.5"
              >
                <div className="flex items-center justify-between gap-2">
                  <code className="text-[10px] text-secondary-foreground">{capability.label}</code>
                  <StatusBadge tone={capability.available ? 'ok' : 'warning'}>
                    {capability.available ? 'Detected' : 'Unavailable'}
                  </StatusBadge>
                </div>
                <p className="mt-1.5 text-[9px] leading-relaxed text-muted-foreground">
                  {capability.detail}
                </p>
              </div>
            ))}
          </div>
          <p className="rounded-md border border-primary/25 bg-primary/10 p-3 text-[10px] leading-relaxed text-primary-text">
            {accounts.privilegeGuidance}
          </p>
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

      <AlertDialog open={rootConfirmationOpen} onOpenChange={setRootConfirmationOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Allow Sentinel to target root?</AlertDialogTitle>
            <AlertDialogDescription>
              This expands the host privilege boundary after restart. Sentinel will not configure
              sudo policy, and every root-targeted action keeps the permissions granted by the host.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="min-h-11">Keep root blocked</AlertDialogCancel>
            <AlertDialogAction className="min-h-11" onClick={() => setRootTargeting(true)}>
              I understand, allow root
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={blocker.status === 'blocked'}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard account changes?</AlertDialogTitle>
            <AlertDialogDescription>
              This targeting draft has not been written to the config file. No OS account has been
              modified.
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

function preserveDraftValue(target: AccountsDraft, source: AccountsDraft, key: AccountsDraftKey) {
  switch (key) {
    case 'allowedUsers':
      target.allowedUsers = source.allowedUsers
      break
    case 'allowRootTarget':
      target.allowRootTarget = source.allowRootTarget
      break
    case 'userSwitchMethod':
      target.userSwitchMethod = source.userSwitchMethod
      break
  }
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

function IdentityDatum({
  label,
  value,
  children,
}: {
  label: string
  value: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">{label}</p>
      <p className="mt-1 truncate font-mono text-[12px] text-secondary-foreground">{value}</p>
      <div className="mt-2">{children}</div>
    </div>
  )
}

function StatusBadge({
  tone,
  children,
}: {
  tone: 'ok' | 'warning' | 'primary' | 'muted'
  children: React.ReactNode
}) {
  return (
    <Badge
      variant="outline"
      className={cn(
        'h-4 px-1.5 text-[8px]',
        tone === 'ok' && 'border-ok/35 bg-ok/10 text-ok-foreground',
        tone === 'warning' && 'border-warning/40 bg-warning/10 text-warning-foreground',
        tone === 'primary' && 'border-primary/30 bg-primary/10 text-primary-text',
        tone === 'muted' && 'border-border-subtle text-muted-foreground',
      )}
    >
      {children}
    </Badge>
  )
}

function ListValueSummary({ setting }: { setting: SettingsResponse['accounts']['allowedUsers'] }) {
  const effective =
    setting.effectiveValue.length === 0
      ? 'All detected accounts'
      : setting.effectiveValue.join(', ')
  const persisted =
    setting.persistedValue == null
      ? 'Inherited'
      : setting.persistedValue.length === 0
        ? 'All detected accounts'
        : setting.persistedValue.join(', ')
  return (
    <dl className="grid gap-1.5 rounded-md border border-border-subtle bg-background/45 p-2.5 text-[10px] sm:grid-cols-2">
      <div className="min-w-0">
        <dt className="uppercase tracking-[0.08em] text-muted-foreground">Effective</dt>
        <dd className="mt-0.5 break-words font-mono text-secondary-foreground">{effective}</dd>
      </div>
      <div className="min-w-0">
        <dt className="uppercase tracking-[0.08em] text-muted-foreground">Persisted</dt>
        <dd className="mt-0.5 break-words font-mono text-secondary-foreground">{persisted}</dd>
      </div>
    </dl>
  )
}
