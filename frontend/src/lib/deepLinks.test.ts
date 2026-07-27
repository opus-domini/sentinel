import { describe, expect, it } from 'vitest'
import {
  metricsSignalSearch,
  parseMetricsSearch,
  parseRunbooksSearch,
  parseServicesSearch,
  parseTmuxSearch,
  runbookDefinitionSearch,
  runbookExecutionSearch,
  serviceLogsSearch,
  serviceStatusSearch,
  tmuxSessionSearch,
} from './deepLinks'

describe('owner deep links', () => {
  it('accepts complete Services targets and preserves valid log time context', () => {
    expect(
      parseServicesSearch({
        service: ' sentinel ',
        panel: 'logs',
        since: '2026-07-27T12:00:00Z',
      }),
    ).toEqual({
      service: 'sentinel',
      panel: 'logs',
      since: '2026-07-27T12:00:00Z',
    })
    expect(
      parseServicesSearch({
        service: 'sentinel',
        panel: 'logs',
        since: '2026-02-30T12:00:00Z',
      }),
    ).toEqual({ service: 'sentinel', panel: 'logs' })
    expect(
      parseServicesSearch({
        service: 'sentinel',
        panel: 'status',
        since: '2026-07-27T12:00:00Z',
      }),
    ).toEqual({ service: 'sentinel', panel: 'status' })
    expect(parseServicesSearch({ service: 'sentinel', panel: 'restart' })).toEqual({})
    expect(parseServicesSearch({ panel: 'status' })).toEqual({})
  })

  it('accepts a definition, an execution, or a combined Runbook target', () => {
    expect(parseRunbooksSearch({ runbook: ' rb ', job: ' run ' })).toEqual({
      runbook: 'rb',
      job: 'run',
    })
    expect(parseRunbooksSearch({ job: ' orphan ' })).toEqual({ job: 'orphan' })
    expect(parseRunbooksSearch({ runbook: ' rb ' })).toEqual({ runbook: 'rb' })
    expect(parseRunbooksSearch({ runbook: '', job: '' })).toEqual({})
  })

  it('normalizes a Tmux session without inventing one', () => {
    expect(parseTmuxSearch({ session: ' dev ' })).toEqual({ session: 'dev' })
    expect(parseTmuxSearch({ session: '' })).toEqual({})
    expect(parseTmuxSearch({ session: 42 })).toEqual({})
  })

  it('accepts only canonical Metrics signals and an optional RFC3339 focus time', () => {
    expect(
      parseMetricsSearch({ signal: 'memoryPressure', focusAt: '2026-07-27T09:00:00-03:00' }),
    ).toEqual({
      signal: 'memoryPressure',
      focusAt: '2026-07-27T09:00:00-03:00',
    })
    expect(parseMetricsSearch({ signal: 'cpu', focusAt: 'yesterday' })).toEqual({
      signal: 'cpu',
    })
    expect(parseMetricsSearch({ signal: 'load' })).toEqual({})
    expect(parseMetricsSearch({ focusAt: '2026-07-27T12:00:00Z' })).toEqual({})
  })

  it('builds canonical owner targets without coupling execution to definition', () => {
    expect(runbookDefinitionSearch('rb')).toEqual({ runbook: 'rb' })
    expect(runbookExecutionSearch('job')).toEqual({ job: 'job' })
    expect(serviceStatusSearch('sentinel')).toEqual({
      service: 'sentinel',
      panel: 'status',
    })
    expect(serviceLogsSearch('sentinel', '2026-07-27T12:00:00Z')).toEqual({
      service: 'sentinel',
      panel: 'logs',
      since: '2026-07-27T12:00:00Z',
    })
    expect(metricsSignalSearch('cpuPressure', '2026-07-27T12:00:00Z')).toEqual({
      signal: 'cpuPressure',
      focusAt: '2026-07-27T12:00:00Z',
    })
    expect(tmuxSessionSearch('dev')).toEqual({ session: 'dev' })
  })
})
