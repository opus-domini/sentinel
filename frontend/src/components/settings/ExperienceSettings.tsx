import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { MonitorCog } from 'lucide-react'

import { SaveFeedback, SettingsField } from './SettingsField'
import SettingsSectionHeader from './SettingsSectionHeader'
import ThemeSelector from './ThemeSelector'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useSettings } from '@/hooks/useSettings'
import { THEME_STORAGE_KEY } from '@/lib/terminalThemes'

type Feedback = {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}

const idleFeedback: Feedback = { status: 'idle', message: '' }

export default function ExperienceSettings() {
  const { timezone, locale } = useMetaContext()
  const { pushToast } = useToastContext()
  const queryClient = useQueryClient()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const timezoneSetting = settings?.experience.timezone
  const localeSetting = settings?.experience.locale
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )
  const [timezoneDraft, setTimezoneDraft] = useState<string | null>(null)
  const [localeDraft, setLocaleDraft] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)

  const effectiveTimezone = timezoneSetting?.effectiveValue ?? timezone
  const effectiveLocale = localeSetting?.effectiveValue ?? locale

  const timezoneOptions = useMemo(() => {
    const options = timezoneSetting?.validation.options ?? []
    if (options.some((option) => option.value === effectiveTimezone)) return options
    return [{ value: effectiveTimezone, label: effectiveTimezone }, ...options]
  }, [effectiveTimezone, timezoneSetting?.validation.options])

  const selectTheme = (id: string) => {
    setThemeId(id)
    localStorage.setItem(THEME_STORAGE_KEY, id)
    window.dispatchEvent(new CustomEvent('sentinel-theme-change', { detail: id }))
    setFeedback({ status: 'success', message: 'Theme applied in this browser.' })
  }

  const saveTimezone = useCallback(
    async (value: string) => {
      setTimezoneDraft(value)
      setFeedback({ status: 'saving', message: 'Saving timezone…' })
      try {
        await settingsQuery.save({ experience: { timezone: value } })
        setTimezoneDraft(null)
        await queryClient.invalidateQueries({ queryKey: ['meta'] })
        setFeedback({ status: 'success', message: 'Timezone saved.' })
        pushToast({
          level: 'success',
          title: 'Timezone saved',
          message: 'Displayed timestamps now use the new timezone.',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to save timezone'
        setFeedback({ status: 'error', message })
        pushToast({ level: 'error', title: 'Timezone not saved', message })
      }
    },
    [pushToast, queryClient, settingsQuery],
  )

  const saveLocale = useCallback(
    async (value: string) => {
      const candidate = value === 'auto' ? '' : value
      setLocaleDraft(candidate)
      setFeedback({ status: 'saving', message: 'Saving date format…' })
      try {
        await settingsQuery.save({ experience: { locale: candidate } })
        setLocaleDraft(null)
        await queryClient.invalidateQueries({ queryKey: ['meta'] })
        setFeedback({ status: 'success', message: 'Date format saved.' })
        pushToast({
          level: 'success',
          title: 'Date format saved',
          message: 'Dates and numbers now use the selected locale.',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to save date format'
        setFeedback({ status: 'error', message })
        pushToast({ level: 'error', title: 'Date format not saved', message })
      }
    },
    [pushToast, queryClient, settingsQuery],
  )

  const loadError =
    settingsQuery.error instanceof Error
      ? settingsQuery.error.message
      : 'Settings could not be loaded.'

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Experience"
        description="Tune how this browser presents Sentinel and how the running server formats operational time."
        icon={<MonitorCog className="size-4" aria-hidden="true" />}
      />

      <SaveFeedback status={feedback.status} message={feedback.message} />

      <SettingsField
        label="Terminal theme"
        description="Choose the terminal palette used by this browser. This preference never changes the server."
        ownership="browser"
      >
        <ThemeSelector activeThemeId={themeId} onSelect={selectTheme} />
      </SettingsField>

      {settingsQuery.isLoading && settings == null && (
        <div
          aria-label="Loading server-managed experience settings"
          className="grid gap-2 rounded-lg border border-border-subtle bg-card p-4"
        >
          <span className="h-4 w-32 motion-safe:animate-pulse rounded bg-surface-active" />
          <span className="h-11 motion-safe:animate-pulse rounded bg-surface-elevated" />
        </div>
      )}

      {settingsQuery.isError && settings == null && (
        <div
          role="alert"
          className="grid gap-3 rounded-lg border border-destructive/45 bg-destructive/10 p-3"
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

      {timezoneSetting && (
        <SettingsField
          label="Timezone"
          description="Choose the timezone used for operational timestamps across the interface."
          setting={timezoneSetting}
          htmlFor="settings-timezone"
        >
          <Select
            value={timezoneDraft ?? effectiveTimezone}
            onValueChange={(value) => void saveTimezone(value)}
            disabled={!timezoneSetting.editable || feedback.status === 'saving'}
          >
            <SelectTrigger
              id="settings-timezone"
              aria-label="Timezone"
              className="min-h-11 w-full bg-surface-overlay sm:max-w-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {timezoneOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!timezoneSetting.editable && (
            <p className="text-[10px] text-warning-foreground">
              This value is owned by the environment and cannot be changed here.
            </p>
          )}
        </SettingsField>
      )}

      {localeSetting && (
        <SettingsField
          label="Date format"
          description="Choose the locale used for dates and numbers across the interface."
          setting={localeSetting}
          htmlFor="settings-locale"
        >
          <Select
            value={(localeDraft ?? effectiveLocale) || 'auto'}
            onValueChange={(value) => void saveLocale(value)}
            disabled={!localeSetting.editable || feedback.status === 'saving'}
          >
            <SelectTrigger
              id="settings-locale"
              aria-label="Date format"
              className="min-h-11 w-full bg-surface-overlay sm:max-w-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {localeSetting.validation.options.map((option) => (
                <SelectItem key={option.value || 'auto'} value={option.value || 'auto'}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!localeSetting.editable && (
            <p className="text-[10px] text-warning-foreground">
              This value is owned by the environment and cannot be changed here.
            </p>
          )}
        </SettingsField>
      )}
    </div>
  )
}
