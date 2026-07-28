import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { MonitorCog } from 'lucide-react'

import { SettingsField, SaveFeedback, ValidationError } from './SettingsField'
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
  const [timezoneDraft, setTimezoneDraft] = useState('')
  const [timezoneTouched, setTimezoneTouched] = useState(false)
  const [timezoneError, setTimezoneError] = useState('')
  const [localeDraft, setLocaleDraft] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<Feedback>(idleFeedback)

  const effectiveTimezone = timezoneSetting?.effectiveValue ?? timezone
  const effectiveLocale = localeSetting?.effectiveValue ?? locale

  useEffect(() => {
    if (!timezoneTouched) setTimezoneDraft(effectiveTimezone)
  }, [effectiveTimezone, timezoneTouched])

  const timezoneOptions = useMemo(() => {
    const options = timezoneSetting?.validation.options ?? []
    return Array.from(new Set(['Local', ...options.map((option) => option.value)])).filter(Boolean)
  }, [timezoneSetting?.validation.options])

  const selectTheme = (id: string) => {
    setThemeId(id)
    localStorage.setItem(THEME_STORAGE_KEY, id)
    window.dispatchEvent(new CustomEvent('sentinel-theme-change', { detail: id }))
    setFeedback({ status: 'success', message: 'Theme applied in this browser.' })
  }

  const saveTimezone = useCallback(async () => {
    const candidate = timezoneDraft.trim()
    const validationMessage = validateTimezone(candidate)
    setTimezoneError(validationMessage)
    if (validationMessage !== '' || candidate === effectiveTimezone) return

    setFeedback({ status: 'saving', message: 'Saving timezone…' })
    try {
      await settingsQuery.save({ experience: { timezone: candidate } })
      setTimezoneTouched(false)
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
  }, [effectiveTimezone, pushToast, queryClient, settingsQuery, timezoneDraft])

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
          description="Use Local or enter a valid IANA timezone, such as America/Sao_Paulo."
          setting={timezoneSetting}
          htmlFor="settings-timezone"
        >
          <form
            className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]"
            onSubmit={(event) => {
              event.preventDefault()
              void saveTimezone()
            }}
          >
            <input
              id="settings-timezone"
              name="timezone"
              type="text"
              list="settings-timezone-options"
              aria-invalid={timezoneError !== ''}
              aria-describedby={timezoneError === '' ? undefined : 'settings-timezone-error'}
              value={timezoneDraft}
              disabled={!timezoneSetting.editable || feedback.status === 'saving'}
              className="min-h-11 min-w-0 rounded-md border border-input bg-surface-overlay px-3 text-[12px] text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              onChange={(event) => {
                setTimezoneDraft(event.target.value)
                setTimezoneTouched(true)
                setTimezoneError('')
              }}
            />
            <datalist id="settings-timezone-options">
              {timezoneOptions.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </datalist>
            <Button
              type="submit"
              className="min-h-11 w-full sm:w-auto"
              disabled={
                !timezoneSetting.editable ||
                feedback.status === 'saving' ||
                timezoneDraft.trim() === effectiveTimezone
              }
            >
              Apply timezone
            </Button>
          </form>
          <ValidationError id="settings-timezone-error" message={timezoneError} />
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

export function validateTimezone(value: string): string {
  if (value === '') return 'Enter Local or a valid IANA timezone.'
  if (value === 'Local') return ''
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: value }).format()
    return ''
  } catch {
    return 'Enter Local or a valid IANA timezone, such as Europe/Lisbon.'
  }
}
