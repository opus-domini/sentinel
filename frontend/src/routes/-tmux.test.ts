import { describe, expect, it } from 'vitest'

import { mapSessionLifecycles } from '@/lib/sessionLifecycle'
import type { Session } from '@/types'

const baseSession: Session = {
  name: 'human',
  windows: 1,
  panes: 1,
  attached: 0,
  createdAt: '2026-08-01T12:00:00Z',
  activityAt: '2026-08-01T12:00:00Z',
  command: 'bash',
  hash: 'hash',
  lastContent: '',
  icon: 'terminal',
}

describe('tmux route lifecycle mapping', () => {
  it('forwards only ephemeral projections to the terminal tab surface', () => {
    const lifecycle = {
      mode: 'ephemeral' as const,
      source: 'mcp' as const,
      cleanupState: 'active' as const,
      expiresAt: '2026-08-01T14:00:00Z',
    }

    const result = mapSessionLifecycles([baseSession, { ...baseSession, name: 'agent', lifecycle }])

    expect(result).toEqual(new Map([['agent', lifecycle]]))
    expect(result.has('human')).toBe(false)
  })
})
