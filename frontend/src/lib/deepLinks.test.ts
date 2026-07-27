import { describe, expect, it } from 'vitest'
import {
  parseRunbooksSearch,
  parseServicesSearch,
  parseTmuxSearch,
  runbookDefinitionSearch,
  runbookExecutionSearch,
  serviceStatusSearch,
  tmuxSessionSearch,
} from './deepLinks'

describe('owner deep links', () => {
  it('accepts only complete Services targets', () => {
    expect(parseServicesSearch({ service: ' sentinel ', panel: 'logs' })).toEqual({
      service: 'sentinel',
      panel: 'logs',
    })
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

  it('builds canonical owner targets without coupling execution to definition', () => {
    expect(runbookDefinitionSearch('rb')).toEqual({ runbook: 'rb' })
    expect(runbookExecutionSearch('job')).toEqual({ job: 'job' })
    expect(serviceStatusSearch('sentinel')).toEqual({
      service: 'sentinel',
      panel: 'status',
    })
    expect(tmuxSessionSearch('dev')).toEqual({ session: 'dev' })
  })
})
