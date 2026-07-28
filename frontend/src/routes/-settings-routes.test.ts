import { describe, expect, it } from 'vitest'

import { guardSettingsSection } from './settings.$section'
import { redirectToExperience } from './settings.index'

describe('settings routes', () => {
  it('redirects the settings index to experience', () => {
    expect(redirectToExperience).toThrow()
  })

  it('accepts only sections delivered by the current workspace', () => {
    expect(() => guardSettingsSection('experience')).not.toThrow()
    expect(() => guardSettingsSection('operations')).not.toThrow()
    expect(() => guardSettingsSection('integrations')).not.toThrow()
    expect(() => guardSettingsSection('storage')).not.toThrow()
    expect(() => guardSettingsSection('diagnostics')).not.toThrow()
    expect(() => guardSettingsSection('accounts')).toThrow()
    expect(() => guardSettingsSection('not-real')).toThrow()
  })
})
