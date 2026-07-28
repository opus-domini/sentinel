import type { SettingsSnapshot } from '@/api/settings'

type SettingsFixtureOptions = {
  revision?: string
  timezone?: string
  locale?: string
  mcpEnabled?: boolean
  tokenConfigured?: boolean
  operations?: Partial<{
    watchtowerEnabled: boolean
    tickInterval: string
    captureLines: number
    captureTimeout: string
    journalRows: number
    maxConcurrent: number
    logLevel: string
  }>
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
      operations: {
        watchtower: {
          enabled: {
            effectiveValue: options.operations?.watchtowerEnabled ?? true,
            defaultValue: true,
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
            },
          },
          tickInterval: {
            effectiveValue: options.operations?.tickInterval ?? '1s',
            defaultValue: '1s',
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              format: 'duration',
              allowCustom: true,
              options: [],
              min: '100ms',
              max: '1m',
            },
          },
          captureLines: {
            effectiveValue: options.operations?.captureLines ?? 80,
            defaultValue: 80,
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              min: 1,
              max: 2000,
              step: 1,
            },
          },
          captureTimeout: {
            effectiveValue: options.operations?.captureTimeout ?? '150ms',
            defaultValue: '150ms',
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              format: 'duration',
              allowCustom: true,
              options: [],
              min: '10ms',
              max: '10s',
            },
          },
          journalRows: {
            effectiveValue: options.operations?.journalRows ?? 5000,
            defaultValue: 5000,
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              min: 100,
              max: 1_000_000,
              step: 1,
            },
          },
        },
        runbooks: {
          maxConcurrent: {
            effectiveValue: options.operations?.maxConcurrent ?? 5,
            defaultValue: 5,
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              min: 1,
              max: 64,
              step: 1,
            },
          },
        },
        log: {
          level: {
            effectiveValue: options.operations?.logLevel ?? 'info',
            defaultValue: 'info',
            source: 'file',
            editable: true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: true,
              allowCustom: false,
              options: [
                { value: 'debug', label: 'Debug' },
                { value: 'info', label: 'Info' },
                { value: 'warn', label: 'Warn' },
                { value: 'error', label: 'Error' },
              ],
            },
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
