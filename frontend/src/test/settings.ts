import type { SettingApplyMode, SettingSource, SettingsSnapshot } from '@/api/settings'

type SettingsFixtureOptions = {
  revision?: string
  timezone?: string
  locale?: string
  mcpEnabled?: boolean
  tokenConfigured?: boolean
  runtimeTokenConfigured?: boolean
  tokenSource?: SettingSource
  tokenEditable?: boolean
  tokenRestartPending?: boolean
  mcpApplyMode?: SettingApplyMode
  mcpRestartPending?: boolean
  healthSchedule?: string
  healthScheduleSource?: SettingSource
  healthScheduleEditable?: boolean
  webhookConfigured?: boolean
  webhookSource?: SettingSource
  webhookEditable?: boolean
  webhookRestartPending?: boolean
  nextActivation?: string
  accounts?: Partial<{
    processUser: string
    processIsRoot: boolean
    inventoryAvailable: boolean
    users: Array<{
      name: string
      processUser: boolean
      root: boolean
      allowed: boolean
    }>
    allowedUsers: Array<string>
    allowedUsersSource: SettingSource
    allowedUsersEditable: boolean
    allowRootTarget: boolean
    allowRootSource: SettingSource
    allowRootEditable: boolean
    userSwitchMethod: string
    methodSource: SettingSource
    methodEditable: boolean
  }>
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
  const runtimeTokenConfigured = options.runtimeTokenConfigured ?? tokenConfigured
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
            applyMode: options.mcpApplyMode ?? 'live',
            restartPending: options.mcpRestartPending ?? false,
            validation: {
              required: true,
            },
          },
          token: {
            configured: tokenConfigured,
            source: options.tokenSource ?? 'file',
            editable: options.tokenEditable ?? true,
            applyMode: 'restart',
            restartPending: options.tokenRestartPending ?? false,
            validation: {
              required: false,
            },
          },
          runtimeTokenConfigured,
          endpoint: '/mcp',
        },
        healthReport: {
          schedule: {
            effectiveValue: options.healthSchedule ?? '',
            defaultValue: '',
            source: options.healthScheduleSource ?? 'file',
            editable: options.healthScheduleEditable ?? true,
            applyMode: 'restart',
            restartPending: false,
            validation: {
              required: false,
              format: 'cron',
              allowCustom: true,
              options: [],
            },
          },
          webhookUrl: {
            configured: options.webhookConfigured ?? false,
            source: options.webhookSource ?? 'file',
            editable: options.webhookEditable ?? true,
            applyMode: 'restart',
            restartPending: options.webhookRestartPending ?? false,
            validation: {
              required: false,
              format: 'url',
            },
          },
          ...(options.nextActivation ? { nextActivation: options.nextActivation } : {}),
        },
      },
      accounts: {
        processUser: options.accounts?.processUser ?? 'hugo',
        processIsRoot: options.accounts?.processIsRoot ?? false,
        inventoryAvailable: options.accounts?.inventoryAvailable ?? true,
        users: options.accounts?.users ?? [
          { name: 'deploy', processUser: false, root: false, allowed: true },
          { name: 'hugo', processUser: true, root: false, allowed: true },
          { name: 'root', processUser: false, root: true, allowed: false },
        ],
        allowedUsers: {
          effectiveValue: options.accounts?.allowedUsers ?? [],
          defaultValue: [],
          source: options.accounts?.allowedUsersSource ?? 'file',
          editable: options.accounts?.allowedUsersEditable ?? true,
          applyMode: 'restart',
          restartPending: false,
          validation: {
            required: false,
            allowCustom: false,
            options: [
              { value: 'deploy', label: 'deploy' },
              { value: 'hugo', label: 'hugo' },
              { value: 'root', label: 'root' },
            ],
          },
        },
        allowRootTarget: {
          effectiveValue: options.accounts?.allowRootTarget ?? false,
          defaultValue: false,
          source: options.accounts?.allowRootSource ?? 'file',
          editable: options.accounts?.allowRootEditable ?? true,
          applyMode: 'restart',
          restartPending: false,
          validation: {
            required: true,
          },
        },
        userSwitchMethod: {
          effectiveValue: options.accounts?.userSwitchMethod ?? 'systemd-run',
          defaultValue: 'systemd-run',
          source: options.accounts?.methodSource ?? 'file',
          editable: options.accounts?.methodEditable ?? true,
          applyMode: 'restart',
          restartPending: false,
          validation: {
            required: true,
            allowCustom: false,
            options: [
              { value: 'sudo', label: 'sudo' },
              { value: 'systemd-run', label: 'systemd-run' },
            ],
          },
        },
        methodCapabilities: [
          {
            value: 'sudo',
            label: 'sudo',
            available: true,
            detail: 'sudo is installed; passwordless policy must still be configured',
          },
          {
            value: 'systemd-run',
            label: 'systemd-run',
            available: true,
            detail:
              'sudo and systemd-run are installed; passwordless policy must still be configured',
          },
        ],
        privilegeGuidance:
          'Sentinel detects executables but cannot grant sudo permissions. Configure passwordless policy outside Sentinel.',
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
