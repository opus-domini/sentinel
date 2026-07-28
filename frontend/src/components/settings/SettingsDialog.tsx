import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import ThemeSelector from '@/components/settings/ThemeSelector'
import MCPSettingsPanel from '@/components/settings/MCPSettingsPanel'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { usePwaInstall } from '@/hooks/usePwaInstall'
import { useSettings } from '@/hooks/useSettings'
import { cn } from '@/lib/utils'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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

type SettingsSection = 'appearance' | 'app' | 'mcp' | 'data' | 'about'

export default function SettingsDialog({ open, onOpenChange }: SettingsDialogProps) {
  const { version, timezone, locale, hostname } = useMetaContext()
  const queryClient = useQueryClient()
  const [savingTimezone, setSavingTimezone] = useState(false)
  const [savingLocale, setSavingLocale] = useState(false)
  const [timezoneDraft, setTimezoneDraft] = useState<string | null>(null)
  const [localeDraft, setLocaleDraft] = useState<string | null>(null)
  const [experienceSaveError, setExperienceSaveError] = useState('')
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )
  const [activeSection, setActiveSection] = useState<SettingsSection>('appearance')
  const {
    supportsPwa,
    installed,
    installAvailable,
    installApp,
    updateAvailable,
    checkForUpdate,
    updating,
  } = usePwaInstall()
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings({ enabled: open })
  const settings = settingsQuery.settings
  const timezoneSetting = settings?.experience.timezone
  const localeSetting = settings?.experience.locale
  const selectedTimezone = timezoneDraft ?? timezoneSetting?.effectiveValue ?? timezone
  const selectedLocale = localeDraft ?? localeSetting?.effectiveValue ?? locale

  useEffect(() => {
    if (!open) {
      setTimezoneDraft(null)
      setLocaleDraft(null)
      setExperienceSaveError('')
    }
  }, [open])

  const handleUpdateApp = useCallback(async () => {
    const result = await checkForUpdate()
    if (result === 'applied') {
      // Controller change listener will reload automatically.
      return
    }
    if (result === 'no-update') {
      // Nothing new to install — reload anyway so the user sees a
      // refreshed surface. Covers the case where an asset cache is
      // stale but the SW bytes match.
      window.location.reload()
      return
    }
    pushToast({
      level: 'error',
      title: 'Update unavailable',
      message:
        result === 'unsupported'
          ? 'Service workers require HTTPS or localhost.'
          : 'Failed to reach the update channel. Check connectivity.',
    })
  }, [checkForUpdate, pushToast])

  const selectTheme = (id: string) => {
    setThemeId(id)
    localStorage.setItem(THEME_STORAGE_KEY, id)
    window.dispatchEvent(new CustomEvent('sentinel-theme-change', { detail: id }))
  }

  const changeTimezone = useCallback(
    async (tz: string) => {
      setTimezoneDraft(tz)
      setSavingTimezone(true)
      setExperienceSaveError('')
      try {
        await settingsQuery.save({
          experience: { timezone: tz },
        })
        setTimezoneDraft(null)
        await queryClient.invalidateQueries({ queryKey: ['meta'] })
        pushToast({
          level: 'success',
          title: 'Timezone saved',
          message: 'Displayed timestamps now use the new timezone.',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to save timezone'
        setExperienceSaveError(message)
        pushToast({
          level: 'error',
          title: 'Timezone not saved',
          message,
        })
      } finally {
        setSavingTimezone(false)
      }
    },
    [pushToast, queryClient, settingsQuery],
  )

  const changeLocale = useCallback(
    async (loc: string) => {
      setLocaleDraft(loc)
      setSavingLocale(true)
      setExperienceSaveError('')
      try {
        await settingsQuery.save({
          experience: { locale: loc },
        })
        setLocaleDraft(null)
        await queryClient.invalidateQueries({ queryKey: ['meta'] })
        pushToast({
          level: 'success',
          title: 'Date format saved',
          message: 'Dates and numbers now use the selected locale.',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to save date format'
        setExperienceSaveError(message)
        pushToast({
          level: 'error',
          title: 'Date format not saved',
          message,
        })
      } finally {
        setSavingLocale(false)
      }
    },
    [pushToast, queryClient, settingsQuery],
  )

  const sectionButtonClass = (section: SettingsSection) =>
    cn(
      'rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors',
      activeSection === section
        ? 'bg-primary/15 text-primary-text'
        : 'text-muted-foreground hover:bg-surface-overlay hover:text-foreground',
    )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="inset-0 flex h-dvh max-h-none w-full max-w-none translate-x-0 translate-y-0 flex-col overflow-x-hidden rounded-none sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-2xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:min-h-[680px]">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
          <DialogDescription>Configure your Sentinel experience.</DialogDescription>
        </DialogHeader>

        <div className="mt-1 grid min-h-0 min-w-0 flex-1 grid-rows-[auto_1fr] gap-3">
          <div
            className="flex flex-wrap gap-1 rounded-md border border-border-subtle bg-secondary p-1"
            role="tablist"
          >
            <button
              type="button"
              role="tab"
              id="settings-tab-appearance"
              aria-selected={activeSection === 'appearance'}
              aria-controls="settings-panel-appearance"
              className={sectionButtonClass('appearance')}
              onClick={() => setActiveSection('appearance')}
            >
              Appearance
            </button>
            <button
              type="button"
              role="tab"
              id="settings-tab-app"
              aria-selected={activeSection === 'app'}
              aria-controls="settings-panel-app"
              className={sectionButtonClass('app')}
              onClick={() => setActiveSection('app')}
            >
              App
            </button>
            <button
              type="button"
              role="tab"
              id="settings-tab-mcp"
              aria-selected={activeSection === 'mcp'}
              aria-controls="settings-panel-mcp"
              className={sectionButtonClass('mcp')}
              onClick={() => setActiveSection('mcp')}
            >
              MCP
            </button>
            <button
              type="button"
              role="tab"
              id="settings-tab-data"
              aria-selected={activeSection === 'data'}
              aria-controls="settings-panel-data"
              className={sectionButtonClass('data')}
              onClick={() => setActiveSection('data')}
            >
              Data
            </button>
            <button
              type="button"
              role="tab"
              id="settings-tab-about"
              aria-selected={activeSection === 'about'}
              aria-controls="settings-panel-about"
              className={sectionButtonClass('about')}
              onClick={() => setActiveSection('about')}
            >
              About
            </button>
          </div>

          {activeSection === 'appearance' && (
            <section
              id="settings-panel-appearance"
              role="tabpanel"
              aria-labelledby="settings-tab-appearance"
              className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border-subtle bg-secondary p-3"
            >
              <h3 className="mb-1 text-xs font-medium">Terminal Theme</h3>
              <p className="mb-3 text-xs text-muted-foreground">
                Choose a color theme for the terminal emulator.
              </p>
              <ThemeSelector activeThemeId={themeId} onSelect={selectTheme} />
            </section>
          )}

          {activeSection === 'app' && (
            <section
              id="settings-panel-app"
              role="tabpanel"
              aria-labelledby="settings-tab-app"
              className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border-subtle bg-secondary p-3"
            >
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
                  onClick={() => {
                    void handleUpdateApp()
                  }}
                  disabled={!supportsPwa || updating}
                  title={
                    updateAvailable
                      ? 'A new version is ready — tap to activate and reload.'
                      : 'Check the server for a new version and reload.'
                  }
                >
                  {updating ? 'Updating...' : updateAvailable ? 'Apply Update' : 'Check for Update'}
                </Button>
                {!supportsPwa && (
                  <span className="text-[11px] text-warning-foreground">
                    PWA needs HTTPS or localhost.
                  </span>
                )}
              </div>

              <div className="mt-5 border-t border-border-subtle pt-4">
                {settingsQuery.isLoading && (
                  <p className="mb-3 text-xs text-muted-foreground">Loading settings…</p>
                )}
                {settingsQuery.error instanceof Error && settings == null && (
                  <div className="mb-3 rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
                    {settingsQuery.error.message}
                  </div>
                )}
                {experienceSaveError !== '' && (
                  <div className="mb-3 rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
                    {experienceSaveError}
                  </div>
                )}
                <h3 className="mb-1 text-xs font-medium">Timezone</h3>
                <p className="mb-2 text-xs text-muted-foreground">
                  IANA timezone used for all displayed timestamps.
                </p>
                <Select
                  value={selectedTimezone}
                  onValueChange={(v) => void changeTimezone(v)}
                  disabled={
                    savingTimezone ||
                    settingsQuery.isLoading ||
                    settings == null ||
                    timezoneSetting?.editable === false
                  }
                >
                  <SelectTrigger className="w-full max-w-xs bg-surface-overlay text-[12px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(timezoneSetting?.validation.options ?? []).map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="mt-5 border-t border-border-subtle pt-4">
                <h3 className="mb-1 text-xs font-medium">Date Format</h3>
                <p className="mb-2 text-xs text-muted-foreground">
                  Locale used for date and number formatting.
                </p>
                <Select
                  value={selectedLocale || 'auto'}
                  onValueChange={(v) => void changeLocale(v === 'auto' ? '' : v)}
                  disabled={
                    savingLocale ||
                    settingsQuery.isLoading ||
                    settings == null ||
                    localeSetting?.editable === false
                  }
                >
                  <SelectTrigger className="w-full max-w-xs bg-surface-overlay text-[12px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(localeSetting?.validation.options ?? []).map((option) => (
                      <SelectItem key={option.value || 'auto'} value={option.value || 'auto'}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </section>
          )}

          {activeSection === 'data' && (
            <section
              id="settings-panel-data"
              role="tabpanel"
              aria-labelledby="settings-tab-data"
              className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border-subtle bg-secondary p-3"
            >
              <div className="grid gap-3">
                <div>
                  <h3 className="text-xs font-medium">Data & Storage</h3>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                    Review persisted history, protected active jobs, and destructive cleanup in the
                    dedicated maintenance workspace.
                  </p>
                </div>
                <div className="rounded-md border border-border-subtle bg-surface-overlay p-3">
                  <p className="text-[11px] font-medium text-foreground">
                    Active jobs are always preserved
                  </p>
                  <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
                    Queued, running, and approval-waiting executions cannot be removed by storage
                    cleanup.
                  </p>
                </div>
                <Button asChild variant="outline" className="min-h-11 w-full sm:w-fit">
                  <Link to="/maintenance/storage" onClick={() => onOpenChange(false)}>
                    Open storage maintenance
                  </Link>
                </Button>
              </div>
            </section>
          )}

          {activeSection === 'mcp' && (
            <section
              id="settings-panel-mcp"
              role="tabpanel"
              aria-labelledby="settings-tab-mcp"
              className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border-subtle bg-secondary p-3"
            >
              <MCPSettingsPanel hostname={hostname} />
            </section>
          )}

          {activeSection === 'about' && (
            <section
              id="settings-panel-about"
              role="tabpanel"
              aria-labelledby="settings-tab-about"
              className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border-subtle bg-secondary p-3"
            >
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
