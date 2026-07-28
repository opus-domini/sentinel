import SecretSettingControl from './SecretSettingControl'
import type { SecretIntent } from './SecretSettingControl'
import {
  EnvironmentOwnership,
  SettingsField,
  SettingValueSummary,
  ValidationError,
} from './SettingsField'
import type { SettingsResponse } from '@/api/settings'
import { Input } from '@/components/ui/input'

type HealthReportSettingsPanelProps = {
  settings: SettingsResponse['integrations']['healthReport']
  schedule: string
  scheduleChanged: boolean
  webhookIntent: SecretIntent
  webhookValue: string
  scheduleError?: string
  webhookError?: string
  onScheduleChange: (value: string) => void
  onWebhookIntentChange: (intent: SecretIntent) => void
  onWebhookValueChange: (value: string) => void
}

export default function HealthReportSettingsPanel({
  settings,
  schedule,
  scheduleChanged,
  webhookIntent,
  webhookValue,
  scheduleError = '',
  webhookError = '',
  onScheduleChange,
  onWebhookIntentChange,
  onWebhookValueChange,
}: HealthReportSettingsPanelProps) {
  const scheduleID = 'settings-health-report-schedule'
  const scheduleErrorID = `${scheduleID}-error`
  return (
    <div className="grid gap-3">
      <SettingsField
        label="Delivery schedule"
        description="Use the server's five-field cron grammar or a supported @descriptor. Empty disables scheduled delivery."
        htmlFor={scheduleID}
        setting={settings.schedule}
      >
        <Input
          id={scheduleID}
          value={schedule}
          placeholder="0 9 * * 1-5"
          autoCapitalize="none"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={scheduleError !== '' ? true : undefined}
          aria-describedby={scheduleError !== '' ? scheduleErrorID : `${scheduleID}-help`}
          disabled={!settings.schedule.editable}
          className="min-h-11 bg-surface-overlay font-mono sm:max-w-md"
          onChange={(event) => onScheduleChange(event.target.value)}
        />
        <p id={`${scheduleID}-help`} className="text-[10px] leading-relaxed text-muted-foreground">
          Cron is parsed only by Sentinel. Save a change to calculate its next activation.
        </p>
        <ValidationError id={scheduleErrorID} message={scheduleError} />
        <EnvironmentOwnership setting={settings.schedule} />
        <SettingValueSummary setting={settings.schedule} />
        <div className="rounded-md border border-border-subtle bg-background/45 px-3 py-2 text-[10px]">
          <span className="uppercase tracking-[0.08em] text-muted-foreground">Next activation</span>
          <p className="mt-1 break-words font-mono text-secondary-foreground">
            {scheduleChanged
              ? 'Save to calculate'
              : settings.nextActivation
                ? settings.nextActivation
                : settings.webhookUrl.configured && settings.schedule.effectiveValue
                  ? 'Unavailable'
                  : 'Inactive until both schedule and webhook are configured'}
          </p>
        </div>
      </SettingsField>

      <SettingsField
        label="Delivery webhook"
        description="Send health reports to one HTTP(S) endpoint. The URL is write-only because its path or query may contain a credential."
        setting={settings.webhookUrl}
      >
        <SecretSettingControl
          id="settings-health-report-webhook"
          label="Webhook URL"
          setting={settings.webhookUrl}
          intent={webhookIntent}
          value={webhookValue}
          error={webhookError}
          placeholder="https://hooks.example.test/sentinel"
          onIntentChange={onWebhookIntentChange}
          onValueChange={onWebhookValueChange}
        />
      </SettingsField>
    </div>
  )
}
