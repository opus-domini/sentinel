// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import WindowStrip from './WindowStrip'
import type { TmuxLauncher, WindowInfo } from '@/types'
import { TooltipProvider } from '@/components/ui/tooltip'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

describe('WindowStrip', () => {
  afterEach(() => {
    cleanup()
  })

  function openLauncherMenu() {
    fireEvent.pointerDown(
      screen.getByRole('button', { name: 'Open launcher menu' }),
      { button: 0, ctrlKey: false },
    )
  }

  function renderStrip(props: Parameters<typeof WindowStrip>[0]) {
    return render(
      <TooltipProvider>
        <WindowStrip {...props} />
      </TooltipProvider>,
    )
  }

  it('keeps blank window creation on the primary button', () => {
    const onCreateWindow = vi.fn()

    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [],
      activeWindowIndex: null,
      launchers: [],
      recentLauncher: null,
      onSelectWindow: vi.fn(),
      onCloseWindow: vi.fn(),
      onRenameWindow: vi.fn(),
      onCreateWindow,
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create blank window' }))

    expect(onCreateWindow).toHaveBeenCalledTimes(1)
  })

  it('launches configured launchers from the dropdown menu', async () => {
    const onLaunchLauncher = vi.fn()
    const onOpenLaunchers = vi.fn()
    const windows: Array<WindowInfo> = [
      {
        session: 'dev',
        index: 0,
        name: 'shell',
        displayName: 'shell',
        active: true,
        panes: 1,
      },
    ]
    const launchers: Array<TmuxLauncher> = [
      {
        id: 'launcher-codex',
        name: 'Codex',
        icon: 'code',
        command: 'codex',
        cwdMode: 'active-pane',
        cwdValue: '',
        windowName: 'codex',
        sortOrder: 0,
        createdAt: '2026-03-31T12:00:00Z',
        updatedAt: '2026-03-31T12:00:00Z',
        lastUsedAt: '2026-03-31T13:00:00Z',
      },
      {
        id: 'launcher-claude',
        name: 'Claude Code',
        icon: 'bot',
        command: 'claude',
        cwdMode: 'active-pane',
        cwdValue: '',
        windowName: 'claude',
        sortOrder: 1,
        createdAt: '2026-03-31T12:00:00Z',
        updatedAt: '2026-03-31T12:00:00Z',
        lastUsedAt: '',
      },
    ]

    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows,
      activeWindowIndex: 0,
      launchers,
      recentLauncher: launchers[0],
      onSelectWindow: vi.fn(),
      onCloseWindow: vi.fn(),
      onRenameWindow: vi.fn(),
      onCreateWindow: vi.fn(),
      onLaunchLauncher,
      onOpenLaunchers,
    })

    openLauncherMenu()
    fireEvent.click(await screen.findByText('Codex'))

    await waitFor(() => {
      expect(onLaunchLauncher).toHaveBeenCalledWith('launcher-codex')
    })

    openLauncherMenu()
    fireEvent.click(await screen.findByText('Manage launchers...'))

    await waitFor(() => {
      expect(onOpenLaunchers).toHaveBeenCalledTimes(1)
    })
  })

  it('renders malformed launcher commands as plain shell without crashing', async () => {
    const launchers: Array<TmuxLauncher> = [
      {
        id: 'launcher-runner',
        name: 'Runner',
        icon: 'terminal',
        command: undefined as unknown as string,
        cwdMode: 'session',
        cwdValue: '',
        windowName: 'runner',
        sortOrder: 0,
        createdAt: '2026-03-31T12:00:00Z',
        updatedAt: '2026-03-31T12:00:00Z',
        lastUsedAt: '',
      },
    ]

    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'runner',
          displayName: undefined as unknown as string,
          active: true,
          panes: 1,
        },
      ],
      activeWindowIndex: 0,
      launchers,
      recentLauncher: launchers[0],
      onSelectWindow: vi.fn(),
      onCloseWindow: vi.fn(),
      onRenameWindow: vi.fn(),
      onCreateWindow: vi.fn(),
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
    })

    expect(screen.getByText('runner')).not.toBeNull()

    openLauncherMenu()
    expect(await screen.findAllByText('plain shell')).toHaveLength(1)
  })

  it('prefers the last valid window snapshot over transient loading and error states', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: true,
      inspectorError: 'Failed to fetch',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'runner',
          displayName: 'Runner',
          active: true,
          panes: 1,
        },
      ],
      activeWindowIndex: 0,
      launchers: [],
      recentLauncher: null,
      onSelectWindow: vi.fn(),
      onCloseWindow: vi.fn(),
      onRenameWindow: vi.fn(),
      onCreateWindow: vi.fn(),
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
    })

    expect(screen.getByText('Runner')).not.toBeNull()
    expect(screen.queryByText('Failed to fetch')).toBeNull()
    expect(screen.queryByText('Loading windows')).toBeNull()
  })
})
