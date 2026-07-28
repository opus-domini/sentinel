import type { SettingsSnapshot } from '@/api/settings'

type SettingsFixtureOptions = {
  revision?: string
  timezone?: string
  locale?: string
  mcpEnabled?: boolean
  tokenConfigured?: boolean
}

export function createSettingsSnapshot(options: SettingsFixtureOptions = {}): SettingsSnapshot {
  const revision = options.revision ?? 'a'.repeat(64)
  const timezone = options.timezone ?? 'UTC'
  const locale = options.locale ?? 'en-US'
  const mcpEnabled = options.mcpEnabled ?? false
  const tokenConfigured = options.tokenConfigured ?? true
  return {
    etag: `"${revision}"`,
    settings: {
      revision,
      metadata: {
        version: 'test',
      },
      deployment: {
        scope: 'standalone',
        runtimeMode: 'standalone',
        configPath: '/tmp/sentinel/config.toml',
      },
      restart: {
        required: false,
        changedKeys: [],
        backupPath: '/tmp/sentinel/config.toml.bak',
        instruction: 'Restart Sentinel with the external supervisor that owns this process.',
      },
      experience: {
        timezone: {
          effectiveValue: timezone,
          defaultValue: 'Local',
          source: 'file',
          editable: true,
          applyMode: 'partial',
          restartPending: false,
          validation: {
            required: true,
            format: 'iana-timezone',
            allowCustom: true,
            options: [
              { value: 'Local', label: 'Local' },
              { value: 'UTC', label: 'UTC' },
              { value: 'America/Sao_Paulo', label: 'America/Sao_Paulo' },
            ],
          },
        },
        locale: {
          effectiveValue: locale,
          defaultValue: '',
          source: 'file',
          editable: true,
          applyMode: 'live',
          restartPending: false,
          validation: {
            required: false,
            format: 'bcp-47',
            allowCustom: false,
            options: [
              { value: '', label: 'Browser default' },
              { value: 'en-US', label: 'English (US)' },
              { value: 'pt-BR', label: 'Português (Brasil)' },
            ],
          },
        },
      },
      integrations: {
        mcp: {
          enabled: {
            effectiveValue: mcpEnabled,
            defaultValue: false,
            source: 'file',
            editable: true,
            applyMode: 'live',
            restartPending: false,
            validation: {
              required: true,
            },
          },
          tokenConfigured,
          endpoint: '/mcp',
        },
      },
      diagnostics: {
        configExists: true,
        environmentOwnedKeys: [],
        readOnlyKeys: ['version', 'storage.path', 'log.path'],
        deploymentDetection: 'standalone',
      },
    },
  }
}
