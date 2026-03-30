// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionSidebar from './SessionSidebar'

vi.mock('./sidebar/SidebarShell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('./sidebar/SessionControls', () => ({
  default: () => <div>Session Controls</div>,
}))

vi.mock('./sidebar/PinnedSessionsPanel', () => ({
  default: ({ fillHeight }: { fillHeight?: boolean }) => (
    <div>{fillHeight ? 'Pinned Fill' : 'Pinned'}</div>
  ),
}))

vi.mock('./sidebar/SessionListPanel', () => ({
  default: () => <div>Session List</div>,
}))

afterEach(() => {
  cleanup()
})

const baseProps = {
  sessions: [],
  totalSessions: 0,
  openTabs: [],
  activeSession: '',
  isOpen: true,
  collapsed: false,
  tokenRequired: false,
  authenticated: true,
  defaultCwd: '/tmp',
  presets: [],
  filter: '',
  tmuxUnavailable: false,
  onFilterChange: () => {},
  onTokenChange: () => {},
  onCreate: () => {},
  onPinSession: () => {},
  onUnpinSession: () => {},
  onLaunchPreset: () => {},
  onReorderPinned: () => {},
  onReorderSession: () => {},
  onAttach: () => {},
  onRename: () => {},
  onDetach: () => {},
  onKill: () => {},
  onChangeIcon: () => {},
}

describe('SessionSidebar', () => {
  it('hides the regular session panel when every visible session is pinned', () => {
    render(
      <SessionSidebar
        {...baseProps}
        sessions={[
          {
            name: 'api',
            windows: 1,
            panes: 1,
            attached: 1,
            createdAt: '2026-03-30T00:00:00Z',
            activityAt: '2026-03-30T00:00:00Z',
            command: 'bash',
            hash: 'hash-api',
            lastContent: 'ready',
            icon: 'server',
          },
        ]}
        totalSessions={1}
        presets={[
          {
            name: 'api',
            cwd: '/srv/api',
            icon: 'server',
            createdAt: '2026-03-30T00:00:00Z',
            updatedAt: '2026-03-30T00:00:00Z',
            lastLaunchedAt: '',
            launchCount: 0,
          },
        ]}
      />,
    )

    expect(screen.getByText('Pinned Fill')).toBeTruthy()
    expect(screen.queryByText('Session List')).toBeNull()
  })

  it('keeps the regular session panel when there are non-pinned sessions', () => {
    render(
      <SessionSidebar
        {...baseProps}
        sessions={[
          {
            name: 'web',
            windows: 1,
            panes: 1,
            attached: 1,
            createdAt: '2026-03-30T00:00:00Z',
            activityAt: '2026-03-30T00:00:00Z',
            command: 'bash',
            hash: 'hash-web',
            lastContent: 'ready',
            icon: 'terminal',
          },
        ]}
        totalSessions={1}
      />,
    )

    expect(screen.getByText('Pinned')).toBeTruthy()
    expect(screen.getByText('Session List')).toBeTruthy()
  })
})
