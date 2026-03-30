// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionListPanel from './SessionListPanel'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatTimestamp: (value: string) => value,
  }),
}))

afterEach(() => {
  cleanup()
})

const baseSession = {
  windows: 1,
  panes: 1,
  attached: 0,
  createdAt: '2026-03-29T00:00:00Z',
  activityAt: '2026-03-29T00:00:00Z',
  command: 'bash',
  hash: 'abcdef123456',
  lastContent: 'ready',
  icon: 'terminal',
}

describe('SessionListPanel', () => {
  it('excludes pinned sessions from the attached and idle list', () => {
    render(
      <SessionListPanel
        sessions={[
          { ...baseSession, name: 'api' },
          { ...baseSession, name: 'web' },
        ]}
        tmuxUnavailable={false}
        openTabs={[]}
        activeSession=""
        filter=""
        presets={[
          {
            name: 'api',
            cwd: '/srv/api',
            icon: 'server',
            createdAt: '2026-03-29T00:00:00Z',
            updatedAt: '2026-03-29T00:00:00Z',
            lastLaunchedAt: '',
            launchCount: 0,
          },
        ]}
        onFilterChange={() => {}}
        onAttach={() => {}}
        onRename={() => {}}
        onDetach={() => {}}
        onKill={() => {}}
        onChangeIcon={() => {}}
        onPinSession={() => {}}
        onUnpinSession={() => {}}
        onReorder={() => {}}
      />,
    )

    expect(screen.queryByText('api')).toBeNull()
    expect(screen.getByText('web')).toBeTruthy()
  })
})
