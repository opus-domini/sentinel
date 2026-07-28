import type { SettingsPatch, SettingsResponse } from '@/api/settings'

export type OperationsDraft = {
  watchtowerEnabled: boolean
  tickInterval: string
  captureLines: string
  captureTimeout: string
  journalRows: string
  maxConcurrent: string
  logLevel: string
}

export type OperationsDraftKey = keyof OperationsDraft

export type OperationsDraftChange = {
  key: OperationsDraftKey
  configKey: string
  label: string
  before: string
  after: string
}

export type OperationsDraftErrors = Partial<Record<OperationsDraftKey, string>>

const fieldDetails: Record<
  OperationsDraftKey,
  {
    configKey: string
    label: string
  }
> = {
  watchtowerEnabled: {
    configKey: 'watchtower.enabled',
    label: 'Watchtower',
  },
  tickInterval: {
    configKey: 'watchtower.tick_interval',
    label: 'Collection interval',
  },
  captureLines: {
    configKey: 'watchtower.capture_lines',
    label: 'Capture lines',
  },
  captureTimeout: {
    configKey: 'watchtower.capture_timeout',
    label: 'Capture timeout',
  },
  journalRows: {
    configKey: 'watchtower.journal_rows',
    label: 'Journal rows',
  },
  maxConcurrent: {
    configKey: 'runbooks.max_concurrent',
    label: 'Concurrent runbooks',
  },
  logLevel: {
    configKey: 'log.level',
    label: 'Log level',
  },
}

export function operationsDraftFromSettings(
  operations: SettingsResponse['operations'],
): OperationsDraft {
  return {
    watchtowerEnabled: operations.watchtower.enabled.effectiveValue,
    tickInterval: operations.watchtower.tickInterval.effectiveValue,
    captureLines: String(operations.watchtower.captureLines.effectiveValue),
    captureTimeout: operations.watchtower.captureTimeout.effectiveValue,
    journalRows: String(operations.watchtower.journalRows.effectiveValue),
    maxConcurrent: String(operations.runbooks.maxConcurrent.effectiveValue),
    logLevel: operations.log.level.effectiveValue,
  }
}

export function diffOperationsDraft(
  base: OperationsDraft,
  draft: OperationsDraft,
): Array<OperationsDraftChange> {
  return (Object.keys(fieldDetails) as Array<OperationsDraftKey>)
    .filter((key) => base[key] !== draft[key])
    .map((key) => ({
      key,
      configKey: fieldDetails[key].configKey,
      label: fieldDetails[key].label,
      before: formatDraftValue(base[key]),
      after: formatDraftValue(draft[key]),
    }))
}

export function validateOperationsDraft(
  draft: OperationsDraft,
  operations: SettingsResponse['operations'],
): OperationsDraftErrors {
  const errors: OperationsDraftErrors = {}
  validateDuration(
    errors,
    'tickInterval',
    'Collection interval',
    draft.tickInterval,
    operations.watchtower.tickInterval.validation.min,
    operations.watchtower.tickInterval.validation.max,
  )
  validateInteger(
    errors,
    'captureLines',
    'Capture lines',
    draft.captureLines,
    operations.watchtower.captureLines.validation.min,
    operations.watchtower.captureLines.validation.max,
  )
  validateDuration(
    errors,
    'captureTimeout',
    'Capture timeout',
    draft.captureTimeout,
    operations.watchtower.captureTimeout.validation.min,
    operations.watchtower.captureTimeout.validation.max,
  )
  validateInteger(
    errors,
    'journalRows',
    'Journal rows',
    draft.journalRows,
    operations.watchtower.journalRows.validation.min,
    operations.watchtower.journalRows.validation.max,
  )
  validateInteger(
    errors,
    'maxConcurrent',
    'Concurrent runbooks',
    draft.maxConcurrent,
    operations.runbooks.maxConcurrent.validation.min,
    operations.runbooks.maxConcurrent.validation.max,
  )
  if (!operations.log.level.validation.options.some((option) => option.value === draft.logLevel)) {
    errors.logLevel = 'Choose a log level provided by the server.'
  }
  return errors
}

export function operationsPatchFromChanges(
  draft: OperationsDraft,
  changes: Array<OperationsDraftChange>,
): SettingsPatch {
  const changed = new Set(changes.map((change) => change.key))
  const watchtower: NonNullable<NonNullable<SettingsPatch['operations']>['watchtower']> = {}
  const runbooks: NonNullable<NonNullable<SettingsPatch['operations']>['runbooks']> = {}
  const log: NonNullable<NonNullable<SettingsPatch['operations']>['log']> = {}

  if (changed.has('watchtowerEnabled')) watchtower.enabled = draft.watchtowerEnabled
  if (changed.has('tickInterval')) watchtower.tickInterval = draft.tickInterval.trim()
  if (changed.has('captureLines')) watchtower.captureLines = Number(draft.captureLines)
  if (changed.has('captureTimeout')) watchtower.captureTimeout = draft.captureTimeout.trim()
  if (changed.has('journalRows')) watchtower.journalRows = Number(draft.journalRows)
  if (changed.has('maxConcurrent')) runbooks.maxConcurrent = Number(draft.maxConcurrent)
  if (changed.has('logLevel')) log.level = draft.logLevel

  return {
    operations: {
      ...(Object.keys(watchtower).length > 0 ? { watchtower } : {}),
      ...(Object.keys(runbooks).length > 0 ? { runbooks } : {}),
      ...(Object.keys(log).length > 0 ? { log } : {}),
    },
  }
}

export function durationToMilliseconds(raw: string): number | null {
  const value = raw.trim()
  if (value === '') return null
  const segment = /(\d+(?:\.\d+)?)(ns|us|µs|μs|ms|s|m|h)/gy
  const multipliers: Record<string, number> = {
    ns: 0.000001,
    us: 0.001,
    µs: 0.001,
    μs: 0.001,
    ms: 1,
    s: 1000,
    m: 60_000,
    h: 3_600_000,
  }
  let total = 0
  let position = 0
  while (position < value.length) {
    segment.lastIndex = position
    const match = segment.exec(value)
    if (match == null || match.index !== position) return null
    total += Number(match[1]) * multipliers[match[2]]
    position = segment.lastIndex
  }
  return Number.isFinite(total) && total > 0 ? total : null
}

function validateDuration(
  errors: OperationsDraftErrors,
  key: 'tickInterval' | 'captureTimeout',
  label: string,
  value: string,
  min: string | undefined,
  max: string | undefined,
) {
  const parsed = durationToMilliseconds(value)
  const parsedMin = min == null ? null : durationToMilliseconds(min)
  const parsedMax = max == null ? null : durationToMilliseconds(max)
  if (parsed == null) {
    errors[key] = `${label} must be a duration such as 150ms, 1s, or 1m.`
    return
  }
  if (parsedMin != null && parsed < parsedMin) {
    errors[key] = `${label} must be at least ${min}.`
    return
  }
  if (parsedMax != null && parsed > parsedMax) {
    errors[key] = `${label} must be at most ${max}.`
  }
}

function validateInteger(
  errors: OperationsDraftErrors,
  key: 'captureLines' | 'journalRows' | 'maxConcurrent',
  label: string,
  value: string,
  min: number,
  max: number,
) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed)) {
    errors[key] = `${label} must be a whole number.`
    return
  }
  if (parsed < min || parsed > max) {
    errors[key] = `${label} must be between ${min.toLocaleString()} and ${max.toLocaleString()}.`
  }
}

function formatDraftValue(value: string | boolean): string {
  if (typeof value === 'boolean') return value ? 'Enabled' : 'Disabled'
  return value
}
