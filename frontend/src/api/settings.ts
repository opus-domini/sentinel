import { ApiError } from '@/hooks/useTmuxApi'

export type SettingSource = 'default' | 'file' | 'environment'
export type SettingApplyMode = 'live' | 'partial' | 'restart'

export type SettingOption = {
  value: string
  label: string
}

export type StringSetting = {
  persistedValue?: string
  effectiveValue: string
  defaultValue: string
  source: SettingSource
  editable: boolean
  applyMode: SettingApplyMode
  restartPending: boolean
  validation: {
    required: boolean
    format?: string
    allowCustom: boolean
    options: Array<SettingOption>
    min?: string
    max?: string
  }
}

export type StringListSetting = {
  persistedValue?: Array<string>
  effectiveValue: Array<string>
  defaultValue: Array<string>
  source: SettingSource
  editable: boolean
  applyMode: SettingApplyMode
  restartPending: boolean
  validation: {
    required: boolean
    allowCustom: boolean
    options: Array<SettingOption>
  }
}

export type BooleanSetting = {
  persistedValue?: boolean
  effectiveValue: boolean
  defaultValue: boolean
  source: SettingSource
  editable: boolean
  applyMode: SettingApplyMode
  restartPending: boolean
  validation: {
    required: boolean
  }
}

export type IntegerSetting = {
  persistedValue?: number
  effectiveValue: number
  defaultValue: number
  source: SettingSource
  editable: boolean
  applyMode: SettingApplyMode
  restartPending: boolean
  validation: {
    required: boolean
    min: number
    max: number
    step: number
  }
}

export type SensitiveSetting = {
  configured: boolean
  source: SettingSource
  editable: boolean
  applyMode: SettingApplyMode
  restartPending: boolean
  validation: {
    required: boolean
    format?: string
  }
}

export type SecretMutation =
  | { action: 'keep' }
  | { action: 'replace'; value: string }
  | { action: 'clear' }

export type SettingsResponse = {
  revision: string
  metadata: {
    version: string
  }
  deployment: {
    scope: 'user' | 'system' | 'standalone'
    runtimeMode: 'service' | 'standalone'
    configPath: string
  }
  restart: {
    required: boolean
    changedKeys: Array<string>
    command?: string
    backupPath: string
    instruction: string
  }
  experience: {
    timezone: StringSetting
    locale: StringSetting
  }
  operations: {
    watchtower: {
      enabled: BooleanSetting
      tickInterval: StringSetting
      captureLines: IntegerSetting
      captureTimeout: StringSetting
      journalRows: IntegerSetting
    }
    runbooks: {
      maxConcurrent: IntegerSetting
    }
    log: {
      level: StringSetting
    }
  }
  integrations: {
    mcp: {
      enabled: BooleanSetting
      token: SensitiveSetting
      runtimeTokenConfigured: boolean
      endpoint: string
    }
    healthReport: {
      schedule: StringSetting
      webhookUrl: SensitiveSetting
      nextActivation?: string
    }
  }
  accounts: {
    processUser: string
    processIsRoot: boolean
    inventoryAvailable: boolean
    users: Array<{
      name: string
      processUser: boolean
      root: boolean
      allowed: boolean
    }>
    allowedUsers: StringListSetting
    allowRootTarget: BooleanSetting
    userSwitchMethod: StringSetting
    methodCapabilities: Array<{
      value: string
      label: string
      available: boolean
      detail: string
    }>
    privilegeGuidance: string
  }
  diagnostics: {
    configExists: boolean
    environmentOwnedKeys: Array<string>
    readOnlyKeys: Array<string>
    deploymentDetection: 'matched' | 'standalone' | 'unavailable'
  }
}

export type SettingsPatch = {
  experience?: {
    timezone?: string
    locale?: string
  }
  operations?: {
    watchtower?: {
      enabled?: boolean
      tickInterval?: string
      captureLines?: number
      captureTimeout?: string
      journalRows?: number
    }
    runbooks?: {
      maxConcurrent?: number
    }
    log?: {
      level?: string
    }
  }
  integrations?: {
    mcp?: {
      enabled?: boolean
      token?: SecretMutation
    }
    healthReport?: {
      schedule?: string
      webhookUrl?: SecretMutation
    }
  }
  accounts?: {
    allowedUsers?: Array<string>
    allowRootTarget?: boolean
    userSwitchMethod?: string
  }
}

export type SettingsSnapshot = {
  settings: SettingsResponse
  etag: string
}

type SettingsEnvelope = {
  data?: SettingsResponse
  error?: {
    code?: string
    message?: string
    details?: unknown
  }
}

async function requestSettings(init?: RequestInit): Promise<SettingsSnapshot> {
  const response = await fetch('/api/ops/settings', {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...headersToRecord(init?.headers),
    },
  })
  let payload: SettingsEnvelope = {}
  try {
    payload = (await response.json()) as SettingsEnvelope
  } catch {
    payload = {}
  }
  if (!response.ok) {
    throw new ApiError(
      payload.error?.message ?? `HTTP ${response.status}`,
      response.status,
      payload.error?.code ?? '',
      payload.error?.details,
    )
  }
  if (payload.data == null) {
    throw new ApiError('Settings response did not include data', response.status)
  }
  const etag = response.headers.get('ETag')?.trim() ?? ''
  if (etag === '') {
    throw new ApiError('Settings response did not include an ETag', response.status)
  }
  return { settings: payload.data, etag }
}

function headersToRecord(headers: HeadersInit | undefined): Record<string, string> {
  if (headers == null) return {}
  return Object.fromEntries(new Headers(headers).entries())
}

export function getSettings(): Promise<SettingsSnapshot> {
  return requestSettings()
}

export function patchSettings(etag: string, patch: SettingsPatch): Promise<SettingsSnapshot> {
  return requestSettings({
    method: 'PATCH',
    headers: {
      'If-Match': etag,
    },
    body: JSON.stringify(patch),
  })
}
