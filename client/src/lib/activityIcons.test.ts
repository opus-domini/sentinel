import { describe, expect, it } from 'vitest'
import {
  Activity,
  Bell,
  BookOpen,
  Clock,
  Server,
  Settings,
  Shield,
} from 'lucide-react'
import { ACTIVITY_SOURCES, getActivitySourceIcon } from '@/lib/activityIcons'

describe('getActivitySourceIcon', () => {
  it.each([
    ['runbook', BookOpen],
    ['service', Server],
    ['alert', Bell],
    ['guardrail', Shield],
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
    expect(ACTIVITY_SOURCES).toEqual([
      'runbook',
      'service',
      'alert',
      'guardrail',
      'schedule',
      'config',
    ])
  })
})
