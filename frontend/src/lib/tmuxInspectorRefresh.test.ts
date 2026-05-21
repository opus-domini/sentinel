import { describe, expect, it } from 'vitest'
import { shouldSkipInspectorRefresh } from '@/lib/tmuxInspectorRefresh'

describe('tmuxInspectorRefresh', () => {
  it('skips background refresh while a foreground inspector load is active', () => {
    expect(shouldSkipInspectorRefresh(true, true)).toBe(true)
  })

  it('does not skip foreground refresh', () => {
    expect(shouldSkipInspectorRefresh(false, true)).toBe(false)
    expect(shouldSkipInspectorRefresh(false, false)).toBe(false)
  })

  it('does not skip background refresh when inspector is idle', () => {
    expect(shouldSkipInspectorRefresh(true, false)).toBe(false)
  })
})
