import { describe, expect, it } from 'vitest'

import {
  diffOperationsDraft,
  durationToMilliseconds,
  operationsDraftFromSettings,
  operationsPatchFromChanges,
  validateOperationsDraft,
} from './operationsDraft'
import { createSettingsSnapshot } from '@/test/settings'

describe('operations draft', () => {
  it('builds a typed field diff and the smallest PATCH', () => {
    const operations = createSettingsSnapshot().settings.operations
    const base = operationsDraftFromSettings(operations)
    const draft = {
      ...base,
      tickInterval: '2s',
      maxConcurrent: '8',
    }
    const changes = diffOperationsDraft(base, draft)

    expect(changes.map((change) => change.configKey)).toEqual([
      'watchtower.tick_interval',
      'runbooks.max_concurrent',
    ])
    expect(operationsPatchFromChanges(draft, changes)).toEqual({
      operations: {
        watchtower: { tickInterval: '2s' },
        runbooks: { maxConcurrent: 8 },
      },
    })
  })

  it('validates duration and integer ranges supplied by the backend', () => {
    const operations = createSettingsSnapshot().settings.operations
    const draft = {
      ...operationsDraftFromSettings(operations),
      tickInterval: '99ms',
      captureLines: '2001',
      captureTimeout: 'soon',
      journalRows: '99',
      maxConcurrent: '65',
      logLevel: 'verbose',
    }

    expect(validateOperationsDraft(draft, operations)).toEqual({
      tickInterval: 'Collection interval must be at least 100ms.',
      captureLines: 'Capture lines must be between 1 and 2,000.',
      captureTimeout: 'Capture timeout must be a duration such as 150ms, 1s, or 1m.',
      journalRows: 'Journal rows must be between 100 and 1,000,000.',
      maxConcurrent: 'Concurrent runbooks must be between 1 and 64.',
      logLevel: 'Choose a log level provided by the server.',
    })
  })

  it('parses compact Go-style positive durations used by the API', () => {
    expect(durationToMilliseconds('150ms')).toBe(150)
    expect(durationToMilliseconds('1s')).toBe(1000)
    expect(durationToMilliseconds('1m30s')).toBe(90_000)
    expect(durationToMilliseconds('1h30m0s')).toBe(5_400_000)
    expect(durationToMilliseconds('0s')).toBeNull()
    expect(durationToMilliseconds('soon')).toBeNull()
  })
})
