import { describe, expect, it } from 'vitest'

import {
  diffIntegrationsDraft,
  integrationsDraftFromSettings,
  integrationsErrorsFromAPI,
  integrationsPatchFromChanges,
  validateIntegrationsDraft,
} from './integrationsDraft'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

describe('integrations draft', () => {
  it('builds a typed patch without putting secret values in the diff', () => {
    const integrations = createSettingsSnapshot().settings.integrations
    const base = integrationsDraftFromSettings(integrations)
    const draft = {
      ...base,
      mcpEnabled: true,
      tokenIntent: 'replace' as const,
      tokenValue: 'private-token-value',
      schedule: '0 * * * *',
      webhookIntent: 'replace' as const,
      webhookValue: 'https://hooks.example.test/private',
    }
    const changes = diffIntegrationsDraft(base, draft, integrations)

    expect(JSON.stringify(changes)).not.toContain('private-token-value')
    expect(JSON.stringify(changes)).not.toContain('hooks.example.test')
    expect(integrationsPatchFromChanges(draft, changes)).toEqual({
      integrations: {
        mcp: {
          enabled: true,
          token: { action: 'replace', value: 'private-token-value' },
        },
        healthReport: {
          schedule: '0 * * * *',
          webhookUrl: {
            action: 'replace',
            value: 'https://hooks.example.test/private',
          },
        },
      },
    })
  })

  it('requires replacement values but does not parse cron in the browser', () => {
    const base = integrationsDraftFromSettings(createSettingsSnapshot().settings.integrations)
    expect(
      validateIntegrationsDraft({
        ...base,
        tokenIntent: 'replace',
        webhookIntent: 'replace',
        schedule: 'backend owns this grammar',
      }),
    ).toEqual({
      token: 'Enter a non-empty replacement token.',
      webhook: 'Enter a non-empty replacement webhook URL.',
    })
  })

  it('maps canonical backend issues to fields', () => {
    const error = new ApiError('invalid', 422, 'CONFIG_INVALID', {
      issues: [
        'health_report.schedule invalid cron expression; health_report.webhook_url must be absolute',
      ],
    })
    expect(integrationsErrorsFromAPI(error)).toEqual({
      schedule:
        'health_report.schedule invalid cron expression; health_report.webhook_url must be absolute',
      webhook:
        'health_report.schedule invalid cron expression; health_report.webhook_url must be absolute',
    })
  })
})
