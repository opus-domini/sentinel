import { describe, expect, it } from 'vitest'
import { parseRunbooksSearch, parseServicesSearch, parseTmuxSearch } from './deepLinks'

describe('owner deep links', () => {
  it('accepts only complete Services targets', () => {
    expect(parseServicesSearch({ service: ' sentinel ', panel: 'logs' })).toEqual({
      service: 'sentinel',
      panel: 'logs',
    })
    expect(parseServicesSearch({ service: 'sentinel', panel: 'restart' })).toEqual({})
    expect(parseServicesSearch({ panel: 'status' })).toEqual({})
  })

  it('keeps a Runbook target and optional job', () => {
    expect(parseRunbooksSearch({ runbook: ' rb ', job: ' run ' })).toEqual({
      runbook: 'rb',
      job: 'run',
    })
    expect(parseRunbooksSearch({ job: 'orphan' })).toEqual({})
  })

  it('normalizes a Tmux session without inventing one', () => {
    expect(parseTmuxSearch({ session: ' dev ' })).toEqual({ session: 'dev' })
    expect(parseTmuxSearch({ session: '' })).toEqual({})
    expect(parseTmuxSearch({ session: 42 })).toEqual({})
  })
})
