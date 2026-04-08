import { describe, expect, it, vi } from 'vitest'

import { createTmuxOperationId } from './tmuxOperationId'

describe('createTmuxOperationId', () => {
  it('generates distinct ids for repeated calls', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-08T12:00:00Z'))

    const first = createTmuxOperationId('session-create')
    const second = createTmuxOperationId('session-create')

    expect(first).toBe('session-create-1775649600000-1')
    expect(second).toBe('session-create-1775649600000-2')

    vi.useRealTimers()
  })

  it('falls back to a default prefix when the prefix is blank', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-08T12:00:00Z'))

    expect(createTmuxOperationId('   ')).toMatch(/^tmux-op-1775649600000-\d+$/)

    vi.useRealTimers()
  })
})
