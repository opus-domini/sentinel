// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import WindowStrip, { clampWindowStripTransform } from './WindowStrip'
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
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Open launcher menu' }), {
      button: 0,
      ctrlKey: false,
    })
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
    expect(screen.getByRole('button', { name: 'Create blank window' }).className).toContain(
      'size-5',
    )
    expect(screen.getByRole('button', { name: 'Open launcher menu' }).className).toContain('size-5')
  })

  it('prevents pointer focus from sticking to the blank window button', () => {
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
      onCreateWindow: vi.fn(),
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
    })

    expect(fireEvent.mouseDown(screen.getByRole('button', { name: 'Create blank window' }))).toBe(
      false,
    )
  })

  it('prevents pointer focus from sticking to window tabs and close buttons', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'shell',
          displayName: 'Shell',
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

    expect(fireEvent.mouseDown(screen.getByRole('button', { name: 'Shell' }))).toBe(false)
    expect(fireEvent.mouseDown(screen.getByRole('button', { name: 'Close window #0' }))).toBe(false)
  })

  it('locks window creation, closing, and drag controls without blocking selection', () => {
    const onSelectWindow = vi.fn()
    const onCloseWindow = vi.fn()

    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'one',
          displayName: 'One',
          active: true,
          panes: 1,
          tmuxWindowId: '@1',
        },
        {
          session: 'dev',
          index: 1,
          name: 'two',
          displayName: 'Two',
          active: false,
          panes: 1,
          tmuxWindowId: '@2',
        },
      ],
      activeWindowIndex: 0,
      launchers: [],
      recentLauncher: null,
      onSelectWindow,
      onCloseWindow,
      onRenameWindow: vi.fn(),
      onCreateWindow: vi.fn(),
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
      onReorderWindow: vi.fn(),
    })

    const lockButton = screen.getByRole('button', { name: 'Lock window controls' })
    expect(lockButton.getAttribute('aria-pressed')).toBe('false')
    expect(lockButton.getAttribute('aria-description')).toContain(
      'Prevents creating, closing, and reordering windows.',
    )
    expect(lockButton.className).toContain('cursor-pointer')
    expect(screen.getByRole('button', { name: 'Create blank window' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Open launcher menu' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'One' }).getAttribute('aria-roledescription')).toBe(
      'sortable',
    )
    expect(screen.getByRole('button', { name: 'Close window #0' })).toBeTruthy()

    fireEvent.click(lockButton)

    const unlockButton = screen.getByRole('button', { name: 'Unlock window controls' })
    expect(unlockButton.getAttribute('aria-pressed')).toBe('true')
    expect(unlockButton.getAttribute('aria-description')).toContain(
      'Allows creating, closing, and reordering windows.',
    )
    expect(unlockButton.className).toContain('cursor-pointer')
    expect(screen.queryByRole('button', { name: 'Create blank window' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Open launcher menu' })).toBeNull()
    expect(
      screen.getByRole('button', { name: 'One' }).getAttribute('aria-roledescription'),
    ).toBeNull()
    expect(screen.queryByRole('button', { name: 'Close window #0' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Close window #1' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'One' }))

    expect(onSelectWindow).toHaveBeenCalledWith(0)
    expect(onCloseWindow).not.toHaveBeenCalled()
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

  it('keeps launcher icons and labels on the same visual baseline', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'claude',
          displayName: 'Claude Code',
          displayIcon: 'bot',
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

    expect(screen.getByText('Claude Code').className).toContain('leading-none')
    expect(screen.getByText('Claude Code').className).toContain('pt-[3px]')
  })

  it('keeps window labels vertically aligned when no icon is present', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'shell',
          displayName: 'Shell',
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

    expect(screen.getByText('Shell').className).toContain('pt-[3px]')
  })

  it('keeps active window text neutral while coloring only the icon', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'claude',
          displayName: 'Claude Code',
          displayIcon: 'bot',
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

    const label = screen.getByText('Claude Code')
    const icon = label.previousSibling as SVGElement | null
    const chip = label.closest('div')

    expect(chip?.className).toContain('border-primary/60')
    expect(chip?.className).not.toContain('text-primary-text')
    expect(label.className).not.toContain('text-primary-text')
    expect(icon?.className.baseVal ?? '').toContain('text-primary')
  })

  it('clips vertical overflow while keeping horizontal drag scrolling available', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: [
        {
          session: 'dev',
          index: 0,
          name: 'one',
          displayName: 'One',
          active: true,
          panes: 1,
          tmuxWindowId: '@1',
        },
        {
          session: 'dev',
          index: 1,
          name: 'two',
          displayName: 'Two',
          active: false,
          panes: 1,
          tmuxWindowId: '@2',
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
      onReorderWindow: vi.fn(),
    })

    const stripRoot = screen
      .getByRole('button', { name: 'Create blank window' })
      .closest('div[class*="overflow-x-auto"]')

    expect(stripRoot?.className).toContain('overflow-y-hidden')
    expect(stripRoot?.className).toContain('no-scrollbar')
    expect(stripRoot?.getAttribute('data-sentinel-window-strip-scroll')).toBe('true')
    expect(stripRoot?.getAttribute('style')).toContain('overscroll-behavior-x: contain')
    expect(stripRoot?.getAttribute('style')).toContain('overscroll-behavior-y: none')
  })

  it('maps wheel movement to horizontal strip scrolling', () => {
    renderStrip({
      hasActiveSession: true,
      inspectorLoading: false,
      inspectorError: '',
      windows: Array.from({ length: 10 }, (_, index) => ({
        session: 'dev',
        index,
        name: `win-${index}`,
        displayName: `Window ${index}`,
        active: index === 0,
        panes: 1,
        tmuxWindowId: `@${index + 1}`,
      })),
      activeWindowIndex: 0,
      launchers: [],
      recentLauncher: null,
      onSelectWindow: vi.fn(),
      onCloseWindow: vi.fn(),
      onRenameWindow: vi.fn(),
      onCreateWindow: vi.fn(),
      onLaunchLauncher: vi.fn(),
      onOpenLaunchers: vi.fn(),
      onReorderWindow: vi.fn(),
    })

    const stripRoot = screen
      .getByRole('button', { name: 'Create blank window' })
      .closest('[data-sentinel-window-strip-scroll="true"]') as HTMLDivElement

    Object.defineProperty(stripRoot, 'clientWidth', {
      configurable: true,
      value: 200,
    })
    Object.defineProperty(stripRoot, 'scrollWidth', {
      configurable: true,
      value: 600,
    })

    stripRoot.scrollLeft = 10
    fireEvent.wheel(stripRoot, { deltaY: 48 })

    expect(stripRoot.scrollLeft).toBe(58)
  })

  it('keeps dragged windows inside the visible strip bounds', () => {
    const strip = document.createElement('div')
    strip.dataset.sentinelWindowStripScroll = 'true'
    strip.getBoundingClientRect = () =>
      ({
        top: 0,
        left: 140,
        right: 340,
        bottom: 40,
        width: 200,
        height: 40,
        x: 140,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect

    const clamped = clampWindowStripTransform(
      {
        x: -80,
        y: 18,
        scaleX: 1,
        scaleY: 1,
      },
      {
        top: 8,
        left: 120,
        right: 200,
        bottom: 28,
        width: 80,
        height: 20,
      },
      [strip],
    )

    expect(clamped.x).toBe(20)
    expect(clamped.y).toBe(0)
  })

  it('falls back safely when the strip rect is unavailable', () => {
    const clamped = clampWindowStripTransform(
      {
        x: 32,
        y: 18,
        scaleX: 1,
        scaleY: 1,
      },
      {
        top: 8,
        left: 120,
        right: 200,
        bottom: 28,
        width: 80,
        height: 20,
      },
      [],
    )

    expect(clamped.x).toBe(32)
    expect(clamped.y).toBe(0)
  })
})
