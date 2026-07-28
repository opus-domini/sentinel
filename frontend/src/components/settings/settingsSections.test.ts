import { describe, expect, it } from 'vitest'

import { SETTINGS_SECTIONS, isSettingsSection, settingsSectionFromPath } from './settingsSections'

describe('settings sections', () => {
  it('keeps the delivered section parameter closed', () => {
    expect(SETTINGS_SECTIONS.map((section) => section.id)).toEqual([
      'experience',
      'operations',
      'integrations',
      'storage',
      'diagnostics',
    ])
    expect(isSettingsSection('experience')).toBe(true)
    expect(isSettingsSection('operations')).toBe(true)
    expect(isSettingsSection('accounts')).toBe(false)
  })

  it('derives a safe active section from a deep link', () => {
    expect(settingsSectionFromPath('/settings/storage')).toBe('storage')
    expect(settingsSectionFromPath('/settings/not-real')).toBe('experience')
  })
})
