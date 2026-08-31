import type { SettingsPatch, SettingsResponse } from '@/api/settings'
import type { ApiError } from '@/hooks/useTmuxApi'
import type { SecretIntent } from './SecretSettingControl'
import { readSettingsIssues } from './settingsIssues'

export type IntegrationsDraft = {
  mcpEnabled: boolean
  schedule: string
  webhookIntent: SecretIntent
  webhookValue: string
}

export type IntegrationsDraftKey = 'mcpEnabled' | 'schedule' | 'webhook'

export type IntegrationsDraftChange = {
  key: IntegrationsDraftKey
  configKey: string
  before: string
  after: string
}

export type IntegrationsDraftErrors = Partial<Record<'schedule' | 'webhook', string>>

export function integrationsDraftFromSettings(
  integrations: SettingsResponse['integrations'],
): IntegrationsDraft {
  return {
    mcpEnabled: integrations.mcp.enabled.effectiveValue,
    schedule: integrations.healthReport.schedule.effectiveValue,
    webhookIntent: 'keep',
    webhookValue: '',
  }
}

export function diffIntegrationsDraft(
  base: IntegrationsDraft,
  draft: IntegrationsDraft,
  integrations: SettingsResponse['integrations'],
): Array<IntegrationsDraftChange> {
  const changes: Array<IntegrationsDraftChange> = []
  if (base.mcpEnabled !== draft.mcpEnabled) {
    changes.push({
      key: 'mcpEnabled',
      configKey: 'mcp.enabled',
      before: base.mcpEnabled ? 'Enabled' : 'Disabled',
      after: draft.mcpEnabled ? 'Enabled' : 'Disabled',
    })
  }
  if (base.schedule !== draft.schedule) {
    changes.push({
      key: 'schedule',
      configKey: 'health_report.schedule',
      before: base.schedule || 'Disabled',
      after: draft.schedule || 'Disabled',
    })
  }
  if (draft.webhookIntent !== 'keep') {
    changes.push({
      key: 'webhook',
      configKey: 'health_report.webhook_url',
      before: integrations.healthReport.webhookUrl.configured ? 'Configured' : 'Not configured',
      after: draft.webhookIntent === 'replace' ? 'Replacement staged' : 'Clear',
    })
  }
  return changes
}

export function validateIntegrationsDraft(draft: IntegrationsDraft): IntegrationsDraftErrors {
  const errors: IntegrationsDraftErrors = {}
  if (draft.webhookIntent === 'replace' && draft.webhookValue.trim() === '') {
    errors.webhook = 'Enter a non-empty replacement webhook URL.'
  }
  return errors
}

export function integrationsPatchFromChanges(
  draft: IntegrationsDraft,
  changes: Array<IntegrationsDraftChange>,
): SettingsPatch {
  const changed = new Set(changes.map((change) => change.key))
  const mcp: NonNullable<NonNullable<SettingsPatch['integrations']>['mcp']> = {}
  const healthReport: NonNullable<NonNullable<SettingsPatch['integrations']>['healthReport']> = {}

  if (changed.has('mcpEnabled')) mcp.enabled = draft.mcpEnabled
  if (changed.has('schedule')) healthReport.schedule = draft.schedule.trim()
  if (changed.has('webhook')) {
    healthReport.webhookUrl =
      draft.webhookIntent === 'replace'
        ? { action: 'replace', value: draft.webhookValue.trim() }
        : { action: 'clear' }
  }

  return {
    integrations: {
      ...(Object.keys(mcp).length > 0 ? { mcp } : {}),
      ...(Object.keys(healthReport).length > 0 ? { healthReport } : {}),
    },
  }
}

export function integrationsErrorsFromAPI(error: ApiError): IntegrationsDraftErrors {
  const issues = readSettingsIssues(error.details)
  const result: IntegrationsDraftErrors = {}
  for (const issue of issues) {
    if (issue.includes('health_report.schedule')) result.schedule = issue
    if (issue.includes('health_report.webhook_url')) result.webhook = issue
  }
  return result
}
