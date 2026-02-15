import { useCallback, useEffect, useState } from 'react'
import type {
  GuardrailRule,
  GuardrailRulesResponse,
  StorageFlushResponse,
  StorageStatsResponse,
} from '@/types'

import ThemeSelector from '@/components/settings/ThemeSelector'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { usePwaInstall } from '@/hooks/usePwaInstall'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { cn } from '@/lib/utils'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { THEME_STORAGE_KEY } from '@/lib/terminalThemes'

type SettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type SettingsSection = 'appearance' | 'app' | 'data' | 'security' | 'about'

export default function SettingsDialog({
  open,
  onOpenChange,
}: SettingsDialogProps) {
  const { version } = useMetaContext()
  const { token } = useTokenContext()
  const api = useTmuxApi(token)
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )
  const [guardrailRules, setGuardrailRules] = useState<Array<GuardrailRule>>([])
  const [guardrailLoading, setGuardrailLoading] = useState(false)
  const [guardrailError, setGuardrailError] = useState('')
  const [guardrailSavingID, setGuardrailSavingID] = useState('')
  const [storageStats, setStorageStats] = useState<StorageStatsResponse | null>(null)
  const [storageLoading, setStorageLoading] = useState(false)
  const [storageError, setStorageError] = useState('')
  const [storageNotice, setStorageNotice] = useState('')
  const [storageFlushingResource, setStorageFlushingResource] = useState('')
  const [activeSection, setActiveSection] =
    useState<SettingsSection>('appearance')
  const {
    supportsPwa,
    installed,
    installAvailable,
    installApp,
    updateAvailable,
    applyUpdate,
    updating,
  } = usePwaInstall()

  const selectTheme = (id: string) => {
    setThemeId(id)
    localStorage.setItem(THEME_STORAGE_KEY, id)
    window.dispatchEvent(
      new CustomEvent('sentinel-theme-change', { detail: id }),
    )
  }

  const loadGuardrails = useCallback(async () => {
    setGuardrailLoading(true)
    try {
      const data = await api<GuardrailRulesResponse>('/api/ops/guardrails/rules')
      setGuardrailRules(data.rules)
      setGuardrailError('')
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'failed to load guardrails'
      setGuardrailError(message)
    } finally {
      setGuardrailLoading(false)
    }
  }, [api])

  const loadStorageStats = useCallback(async () => {
    setStorageLoading(true)
    try {
      const data = await api<StorageStatsResponse>('/api/ops/storage/stats')
      setStorageStats(data)
      setStorageError('')
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'failed to load storage stats'
      setStorageError(message)
    } finally {
      setStorageLoading(false)
    }
  }, [api])

  const saveGuardrail = useCallback(
    async (rule: GuardrailRule, patch: Partial<GuardrailRule>) => {
      setGuardrailSavingID(rule.id)
      try {
        await api<GuardrailRulesResponse>(
          `/api/ops/guardrails/rules/${encodeURIComponent(rule.id)}`,
          {
            method: 'PATCH',
            body: JSON.stringify({
              name: patch.name ?? rule.name,
              scope: patch.scope ?? rule.scope,
              pattern: patch.pattern ?? rule.pattern,
              mode: patch.mode ?? rule.mode,
              severity: patch.severity ?? rule.severity,
              message: patch.message ?? rule.message,
              enabled: patch.enabled ?? rule.enabled,
              priority: patch.priority ?? rule.priority,
            }),
          },
        )
        await loadGuardrails()
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to update guardrail'
        setGuardrailError(message)
      } finally {
        setGuardrailSavingID('')
      }
    },
    [api, loadGuardrails],
  )

  const flushStorageResource = useCallback(
    async (resource: string) => {
      const isAll = resource === 'all'
      const accepted = window.confirm(
        isAll
          ? 'Flush all persisted history data? This cannot be undone.'
          : `Flush "${resource}" history data? This cannot be undone.`,
      )
      if (!accepted) return

      setStorageFlushingResource(resource)
      setStorageNotice('')
      try {
        const data = await api<StorageFlushResponse>('/api/ops/storage/flush', {
          method: 'POST',
          body: JSON.stringify({ resource }),
        })
        const removedRows = data.results.reduce((acc, item) => {
          return acc + item.removedRows
        }, 0)
        setStorageNotice(
          `${data.results.length} resource(s) flushed (${removedRows} row(s) removed).`,
        )
        setStorageError('')
        await loadStorageStats()
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to flush storage'
        setStorageError(message)
      } finally {
        setStorageFlushingResource('')
      }
    },
    [api, loadStorageStats],
  )

  useEffect(() => {
    if (!open) return
    if (activeSection !== 'security') return
    void loadGuardrails()
  }, [activeSection, loadGuardrails, open])

  useEffect(() => {
    if (!open) return
    if (activeSection !== 'data') return
    void loadStorageStats()
  }, [activeSection, loadStorageStats, open])

  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value <= 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let size = value
    let index = 0
    while (size >= 1024 && index < units.length - 1) {
      size /= 1024
      index += 1
    }
    const precision = size >= 100 || index === 0 ? 0 : 1
    return `${size.toFixed(precision)} ${units[index]}`
  }

  const formatRows = (value: number) => {
    if (!Number.isFinite(value) || value <= 0) return '0'
    return new Intl.NumberFormat('en-US').format(Math.trunc(value))
  }

  const sectionButtonClass = (section: SettingsSection) =>
    cn(
      'rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors',
      activeSection === section
        ? 'bg-primary/15 text-primary-text'
        : 'text-muted-foreground hover:bg-surface-overlay hover:text-foreground',
    )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex min-w-0 max-w-[calc(100vw-1rem)] flex-col overflow-x-hidden sm:min-h-[680px] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
          <DialogDescription>
            Configure your Sentinel experience.
          </DialogDescription>
        </DialogHeader>

        <div className="mt-1 grid min-h-0 min-w-0 flex-1 grid-rows-[auto_1fr] gap-3">
          <nav className="flex flex-wrap gap-1 rounded-md border border-border-subtle bg-secondary p-1">
            <button
              type="button"
              className={sectionButtonClass('appearance')}
              onClick={() => setActiveSection('appearance')}
            >
              Appearance
            </button>
            <button
              type="button"
              className={sectionButtonClass('app')}
              onClick={() => setActiveSection('app')}
            >
              App
            </button>
            <button
              type="button"
              className={sectionButtonClass('data')}
              onClick={() => setActiveSection('data')}
            >
              Data
            </button>
            <button
              type="button"
              className={sectionButtonClass('security')}
              onClick={() => setActiveSection('security')}
            >
              Security
            </button>
            <button
              type="button"
              className={sectionButtonClass('about')}
              onClick={() => setActiveSection('about')}
            >
              About
            </button>
          </nav>

          {activeSection === 'appearance' && (
            <section className="min-h-0 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-secondary p-3">
              <h3 className="mb-1 text-xs font-medium">Terminal Theme</h3>
              <p className="mb-3 text-xs text-muted-foreground">
                Choose a color theme for the terminal emulator.
              </p>
              <ThemeSelector activeThemeId={themeId} onSelect={selectTheme} />
            </section>
          )}

          {activeSection === 'app' && (
            <section className="min-h-0 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-secondary p-3">
              <div className="mb-1 flex items-center gap-2">
                <h3 className="text-xs font-medium">Progressive App</h3>
                <Badge
                  variant="outline"
                  className={cn(
                    installed
                      ? 'border-ok/45 bg-ok/10 text-ok-foreground'
                      : 'border-border-subtle bg-surface-overlay text-muted-foreground',
                  )}
                >
                  {installed ? 'Installed' : 'Browser'}
                </Badge>
              </div>
              <p className="mb-3 text-xs text-muted-foreground">
                Install Sentinel as an app for faster launch and better mobile UX.
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    void installApp()
                  }}
                  disabled={!installAvailable}
                  title={
                    installAvailable
                      ? 'Install Sentinel'
                      : 'Use browser install menu when available'
                  }
                >
                  Install App
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={applyUpdate}
                  disabled={!updateAvailable || updating}
                >
                  {updating ? 'Updating...' : 'Apply Update'}
                </Button>
                {!supportsPwa && (
                  <span className="text-[11px] text-warning-foreground">
                    PWA needs HTTPS or localhost.
                  </span>
                )}
              </div>
            </section>
          )}

          {activeSection === 'data' && (
            <section className="min-h-0 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-secondary p-3">
              <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-medium">Data & Storage</h3>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      void loadStorageStats()
                    }}
                    disabled={storageLoading}
                  >
                    {storageLoading ? 'Loading...' : 'Refresh'}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      void flushStorageResource('all')
                    }}
                    disabled={
                      storageLoading ||
                      storageFlushingResource !== '' ||
                      storageStats == null
                    }
                  >
                    {storageFlushingResource === 'all'
                      ? 'Flushing...'
                      : 'Flush All'}
                  </Button>
                </div>
              </div>
              <p className="mb-2 text-xs text-muted-foreground">
                Monitor persisted data growth and flush historical resources when
                needed.
              </p>

              {storageError.trim() !== '' && (
                <div className="mb-2 rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
                  {storageError}
                </div>
              )}
              {storageNotice.trim() !== '' && (
                <div className="mb-2 rounded border border-ok/45 bg-ok/10 px-2 py-1 text-[11px] text-ok-foreground">
                  {storageNotice}
                </div>
              )}

              <div className="mb-2 grid gap-2 sm:grid-cols-2">
                <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                  <p className="text-[11px] text-muted-foreground">DB total</p>
                  <p className="font-mono text-[12px] font-semibold text-foreground">
                    {formatBytes(storageStats?.totalBytes ?? 0)}
                  </p>
                </div>
                <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                  <p className="text-[11px] text-muted-foreground">SQLite files</p>
                  <p className="font-mono text-[12px] text-secondary-foreground">
                    db {formatBytes(storageStats?.databaseBytes ?? 0)} · wal{' '}
                    {formatBytes(storageStats?.walBytes ?? 0)} · shm{' '}
                    {formatBytes(storageStats?.shmBytes ?? 0)}
                  </p>
                </div>
              </div>

              <div className="grid min-w-0 gap-2 pr-1">
                {(storageStats?.resources ?? []).map((resource) => (
                  <div
                    key={resource.resource}
                    className="grid min-w-0 gap-2 rounded-md border border-border-subtle bg-surface-overlay p-2 md:grid-cols-[minmax(0,1fr)_7rem_7.5rem_auto] md:items-center"
                  >
                    <div className="min-w-0">
                      <p className="truncate text-[12px] font-medium">
                        {resource.label}
                      </p>
                      <p className="truncate font-mono text-[10px] text-muted-foreground">
                        {resource.resource}
                      </p>
                    </div>
                    <p className="font-mono text-[11px] text-secondary-foreground md:text-right">
                      {formatRows(resource.rows)} rows
                    </p>
                    <p className="font-mono text-[11px] text-secondary-foreground md:text-right">
                      {formatBytes(resource.approxBytes)}
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        void flushStorageResource(resource.resource)
                      }}
                      disabled={
                        storageFlushingResource !== '' ||
                        storageLoading ||
                        resource.rows <= 0
                      }
                    >
                      {storageFlushingResource === resource.resource
                        ? 'Flushing...'
                        : 'Flush'}
                    </Button>
                  </div>
                ))}
                {storageStats != null && storageStats.resources.length === 0 && (
                  <p className="text-[12px] text-muted-foreground">
                    No storage resources available.
                  </p>
                )}
              </div>
            </section>
          )}

          {activeSection === 'security' && (
            <section className="min-h-0 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-secondary p-3">
              <div className="mb-1 flex items-center justify-between gap-2">
                <h3 className="text-xs font-medium">Command Guardrails</h3>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void loadGuardrails()
                  }}
                  disabled={guardrailLoading}
                >
                  {guardrailLoading ? 'Loading...' : 'Refresh'}
                </Button>
              </div>
              <p className="mb-2 text-xs text-muted-foreground">
                Policies evaluated before sensitive operations run.
              </p>
              {guardrailError.trim() !== '' && (
                <div className="mb-2 rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
                  {guardrailError}
                </div>
              )}
              <div className="grid min-w-0 gap-2 pr-1">
                {guardrailRules.map((rule) => (
                  <div
                    key={rule.id}
                    className="min-w-0 overflow-x-hidden rounded-md border border-border-subtle bg-surface-overlay p-2"
                  >
                    <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center">
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-[12px] font-medium">{rule.name}</p>
                        <TooltipHelper content={rule.message || rule.id}>
                          <p className="truncate text-[11px] text-muted-foreground">
                            {rule.message || rule.id}
                          </p>
                        </TooltipHelper>
                      </div>
                      <div className="ml-auto flex w-full max-w-full min-w-0 flex-wrap items-center justify-end gap-2 sm:w-auto md:flex-nowrap">
                        <Badge
                          variant="outline"
                          className="max-w-full justify-center truncate sm:w-[6.5rem]"
                        >
                          {rule.scope}
                        </Badge>
                        <Select
                          value={rule.mode}
                          onValueChange={(value) => {
                            void saveGuardrail(rule, { mode: value })
                          }}
                          disabled={guardrailSavingID === rule.id}
                        >
                          <SelectTrigger className="w-[min(7.25rem,42vw)] sm:w-[8.5rem]">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="allow">allow</SelectItem>
                            <SelectItem value="warn">warn</SelectItem>
                            <SelectItem value="confirm">confirm</SelectItem>
                            <SelectItem value="block">block</SelectItem>
                          </SelectContent>
                        </Select>
                        <button
                          type="button"
                          role="switch"
                          aria-checked={rule.enabled}
                          aria-label={`${rule.name} toggle`}
                          onClick={() => {
                            void saveGuardrail(rule, { enabled: !rule.enabled })
                          }}
                          disabled={guardrailSavingID === rule.id}
                          className={cn(
                            'relative inline-flex h-6 w-11 items-center rounded-full border transition-colors',
                            'focus-visible:ring-ring/40 focus-visible:outline-none focus-visible:ring-2',
                            'disabled:cursor-not-allowed disabled:opacity-60',
                            rule.enabled
                              ? 'border-ok/50 bg-ok/40'
                              : 'border-border-subtle bg-surface-overlay',
                          )}
                        >
                          <span
                            className={cn(
                              'inline-block h-4 w-4 rounded-full bg-white shadow transition-transform',
                              rule.enabled ? 'translate-x-5' : 'translate-x-1',
                            )}
                          />
                        </button>
                      </div>
                    </div>
                    <p className="mt-1 truncate text-[11px] text-muted-foreground">
                      {rule.pattern}
                    </p>
                  </div>
                ))}
                {guardrailRules.length === 0 && !guardrailLoading && (
                  <p className="text-[12px] text-muted-foreground">
                    No guardrail rules found.
                  </p>
                )}
              </div>
            </section>
          )}

          {activeSection === 'about' && (
            <section className="min-h-0 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-secondary p-3">
              <h3 className="mb-1 text-xs font-medium">Sentinel</h3>
              <p className="text-xs text-muted-foreground">
                Version: <span className="font-mono">{version || 'dev'}</span>
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                Runtime mode:{' '}
                <span className="font-medium">
                  {installed ? 'installed app' : 'browser session'}
                </span>
              </p>
            </section>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
