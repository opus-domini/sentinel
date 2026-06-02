import { describe, expect, it } from 'vitest'
import { Activity, BookOpen, Clock, Server, Settings } from 'lucide-react'
import { ACTIVITY_SOURCES, getActivitySourceIcon } from '@/lib/activityIcons'

describe('getActivitySourceIcon', () => {
  it.each([
    ['runbook', BookOpen],
    ['service', Server],
    ['schedule', Clock],
    ['config', Settings],
  ])('maps "%s" to the correct icon', (source, expected) => {
    expect(getActivitySourceIcon(source)).toBe(expected)
  })

  it('returns Activity icon for unknown sources', () => {
    expect(getActivitySourceIcon('unknown')).toBe(Activity)
    expect(getActivitySourceIcon('')).toBe(Activity)
  })
})

describe('ACTIVITY_SOURCES', () => {
  it('contains all expected sources', () => {
    expect(ACTIVITY_SOURCES).toEqual(['runbook', 'service', 'schedule', 'config'])
  })
})
