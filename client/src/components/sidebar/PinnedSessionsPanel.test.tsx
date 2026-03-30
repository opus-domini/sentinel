// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PinnedSessionsPanel from './PinnedSessionsPanel'

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

const pinnedPreset = {
  name: 'api',
  cwd: '/srv/api',
  icon: 'server',
  createdAt: '2026-03-29T00:00:00Z',
  updatedAt: '2026-03-29T00:00:00Z',
  lastLaunchedAt: '',
  launchCount: 0,
}

const pinnedSession = {
  name: 'api',
  windows: 2,
  panes: 3,
  attached: 0,
  createdAt: '2026-03-29T00:00:00Z',
  activityAt: '2026-03-29T00:00:00Z',
  command: 'node',
  hash: 'abcdef123456',
  lastContent: 'ready',
  icon: 'server',
}

describe('PinnedSessionsPanel', () => {
  it('does not render when nothing is pinned', () => {
    const { container } = render(
      <PinnedSessionsPanel
        sessions={[]}
        presets={[]}
        filter=""
        openTabs={[]}
        activeSession=""
        tmuxUnavailable={false}
        onAttach={() => {}}
        onRename={() => {}}
        onDetach={() => {}}
        onKill={() => {}}
        onChangeIcon={() => {}}
        onPinSession={() => {}}
        onUnpinSession={() => {}}
        onLaunchPreset={() => {}}
        onReorder={() => {}}
      />,
    )

    expect(container.innerHTML).toBe('')
  })

  it('renders pinned sessions in their own panel', () => {
    render(
      <PinnedSessionsPanel
        sessions={[pinnedSession]}
        presets={[pinnedPreset]}
        filter=""
        openTabs={[]}
        activeSession=""
        tmuxUnavailable={false}
        onAttach={() => {}}
        onRename={() => {}}
        onDetach={() => {}}
        onKill={() => {}}
        onChangeIcon={() => {}}
        onPinSession={() => {}}
        onUnpinSession={() => {}}
        onLaunchPreset={() => {}}
        onReorder={() => {}}
      />,
    )

    expect(screen.getByText('Pinned')).toBeTruthy()
    expect(screen.getByText('api')).toBeTruthy()
  })

  it('offers a start action when a pinned session is not running', () => {
    const onLaunchPreset = vi.fn()

    render(
      <PinnedSessionsPanel
        sessions={[]}
        presets={[pinnedPreset]}
        filter=""
        openTabs={[]}
        activeSession=""
        tmuxUnavailable={false}
        onAttach={() => {}}
        onRename={() => {}}
        onDetach={() => {}}
        onKill={() => {}}
        onChangeIcon={() => {}}
        onPinSession={() => {}}
        onUnpinSession={() => {}}
        onLaunchPreset={onLaunchPreset}
        onReorder={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /api/i }))

    expect(onLaunchPreset).toHaveBeenCalledWith('api')
  })
})
