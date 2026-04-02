// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionListPanel from './SessionListPanel'

const { useIsMobileLayoutMock } = vi.hoisted(() => ({
  useIsMobileLayoutMock: vi.fn(() => false),
}))

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: useIsMobileLayoutMock,
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatTimestamp: (value: string) => value,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    processUser: 'hugo',
    isRoot: false,
    allowedUsers: ['postgres'],
  }),
}))

afterEach(() => {
  cleanup()
  useIsMobileLayoutMock.mockReturnValue(false)
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

  it('keeps session cards scrollable on mobile', () => {
    useIsMobileLayoutMock.mockReturnValue(true)

    render(
      <SessionListPanel
        sessions={[{ ...baseSession, name: 'web' }]}
        tmuxUnavailable={false}
        openTabs={[]}
        activeSession=""
        filter=""
        presets={[]}
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

    expect(screen.getByRole('button', { name: /web/i }).style.touchAction).toBe(
      'pan-y',
    )
  })

  it('does not create its own vertical scroll container on desktop', () => {
    useIsMobileLayoutMock.mockReturnValue(false)

    const { container } = render(
      <SessionListPanel
        sessions={[{ ...baseSession, name: 'web' }]}
        tmuxUnavailable={false}
        openTabs={[]}
        activeSession=""
        filter=""
        presets={[]}
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

    const section = container.querySelector('section')
    const list = container.querySelector('section > ul')

    expect(section?.className).not.toContain('flex-col')
    expect(section?.className).not.toContain('overflow-hidden')
    expect(list?.className).not.toContain('flex-1')
    expect(list?.className).not.toContain('overflow-y-auto')
  })

  it('lets the outer sidebar own scrolling on mobile', () => {
    useIsMobileLayoutMock.mockReturnValue(true)

    const { container } = render(
      <SessionListPanel
        sessions={[{ ...baseSession, name: 'web' }]}
        tmuxUnavailable={false}
        openTabs={[]}
        activeSession=""
        filter=""
        presets={[]}
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

    const section = container.querySelector('section')
    const list = container.querySelector('section > ul')

    expect(section?.className).not.toContain('h-full')
    expect(section?.className).not.toContain('overflow-hidden')
    expect(list?.className).not.toContain('flex-1')
    expect(list?.className).not.toContain('overflow-y-auto')
  })
})
