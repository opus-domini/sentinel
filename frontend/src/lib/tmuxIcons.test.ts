import { describe, expect, it } from 'vitest'

import { DEFAULT_ICON_KEY, TMUX_ICONS, getTmuxIcon } from './tmuxIcons'

describe('tmuxIcons', () => {
  it('exposes the shared icon catalog for sessions, launchers, and windows', () => {
    expect(TMUX_ICONS.map((entry) => entry.key)).toEqual([
      'bot',
      'debug',
      'code',
      'cloud',
      'database',
      'globe',
      'server',
      'terminal',
    ])
  })

  it('falls back to the default icon when the key is unknown', () => {
    const defaultIcon = getTmuxIcon(DEFAULT_ICON_KEY)

    expect(getTmuxIcon('terminal')).toBe(defaultIcon)
    expect(getTmuxIcon('missing')).toBe(defaultIcon)
  })
})
