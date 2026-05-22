// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import TmuxTerminalPanel from './TmuxTerminalPanel'

const { useIsMobileLayoutMock } = vi.hoisted(() => ({
  useIsMobileLayoutMock: vi.fn(() => false),
}))

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: useIsMobileLayoutMock,
}))

vi.mock('./ConnectionBadge', () => ({
  default: () => <div>Connection Badge</div>,
}))

vi.mock('./SessionTabs', () => ({
  default: () => <div>Session Tabs</div>,
}))

vi.mock('./TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('./terminal/TerminalControls', () => ({
  default: () => <div>Terminal Controls</div>,
}))

vi.mock('./tmux/PaneStrip', () => ({
  default: () => <div>Pane Strip</div>,
}))

vi.mock('./tmux/TerminalHost', () => ({
  default: () => <div>Terminal Host</div>,
}))

vi.mock('./tmux/WindowStrip', () => ({
  default: ({ onCreateWindow }: { onCreateWindow: () => void }) => (
    <button type="button" data-testid="window-strip" onClick={onCreateWindow}>
      Window Strip
    </button>
  ),
}))

const baseProps = {
  connectionState: 'connected' as const,
  statusDetail: 'ready',
  sidebarCollapsed: false,
  openTabs: ['dev', 'ops'],
  activeSession: 'dev',
  inspectorLoading: false,
  inspectorError: '',
  windows: [],
  panes: [],
  activeWindowIndex: null,
  activePaneID: null,
  termCols: 120,
  termRows: 40,
  getTerminalHostRef: () => () => {},
  onToggleSidebarOpen: () => {},
  onSelectWindow: () => {},
  onSelectPane: () => {},
  onRenameWindow: () => {},
  onRenamePane: () => {},
  onCreateWindow: () => {},
  launchers: [],
  recentLauncher: null,
  onLaunchLauncher: () => {},
  onOpenLaunchers: () => {},
  onReorderWindow: () => {},
  onCloseWindow: () => {},
  onSplitPaneVertical: () => {},
  onSplitPaneHorizontal: () => {},
  onClosePane: () => {},
  onRenameTab: () => {},
  onKillTab: () => {},
  onSelectTab: () => {},
  onCloseTab: () => {},
  onReorderTabs: () => {},
  onOpenGuardrails: () => {},
  onOpenTimeline: () => {},
  onOpenCreateSession: () => {},
  onResync: () => {},
}

describe('TmuxTerminalPanel', () => {
  afterEach(() => {
    cleanup()
    useIsMobileLayoutMock.mockReturnValue(false)
  })

  it('hides session tabs on desktop when the sidebar is expanded', () => {
    const { container } = render(<TmuxTerminalPanel {...baseProps} />)

    expect(screen.queryByText('Session Tabs')).toBeNull()
    expect(container.querySelector('main')?.className).toContain('grid-rows-[40px_1fr_28px]')
    expect(screen.getByTestId('window-strip').parentElement?.className).toContain('items-center')
  })

  it('shows session tabs on desktop when the sidebar is collapsed', () => {
    const { container } = render(<TmuxTerminalPanel {...baseProps} sidebarCollapsed />)

    expect(screen.getByText('Session Tabs')).toBeTruthy()
    expect(container.querySelector('main')?.className).toContain('grid-rows-[40px_30px_1fr_28px]')
  })

  it('keeps session tabs visible on mobile even when the sidebar is expanded', () => {
    useIsMobileLayoutMock.mockReturnValue(true)

    render(<TmuxTerminalPanel {...baseProps} sidebarCollapsed={false} />)

    expect(screen.getByText('Session Tabs')).toBeTruthy()
  })

  it('shows a loading overlay while tmux is still attaching', () => {
    render(
      <TmuxTerminalPanel {...baseProps} connectionState="connecting" statusDetail="opening dev" />,
    )

    expect(screen.getByText('Waiting for tmux server')).toBeTruthy()
  })

  it('keeps terminal content visible while recovering the terminal socket', () => {
    render(
      <TmuxTerminalPanel
        {...baseProps}
        connectionState="connecting"
        statusDetail="reconnecting in 2s"
      />,
    )

    expect(screen.queryByText('Reconnecting to tmux')).toBeNull()
    expect(screen.getByText('Terminal Host')).toBeTruthy()
  })

  it('hides the loading overlay once connected', () => {
    render(<TmuxTerminalPanel {...baseProps} />)

    expect(screen.queryByText('Waiting for tmux server')).toBeNull()
    expect(screen.queryByText('Reconnecting to tmux')).toBeNull()
  })

  it('refocuses the terminal after creating a window', () => {
    const onCreateWindow = vi.fn()
    const onFocusTerminal = vi.fn()

    render(
      <TmuxTerminalPanel
        {...baseProps}
        onCreateWindow={onCreateWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    fireEvent.click(screen.getByTestId('window-strip'))

    expect(onCreateWindow).toHaveBeenCalledTimes(1)
    expect(onFocusTerminal).toHaveBeenCalled()
  })

  it('creates a window with Ctrl+T and keeps browser focus in the terminal', () => {
    const onCreateWindow = vi.fn()
    const onFocusTerminal = vi.fn()

    render(
      <TmuxTerminalPanel
        {...baseProps}
        onCreateWindow={onCreateWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    expect(fireEvent.keyDown(document, { key: 't', ctrlKey: true })).toBe(false)
    expect(onCreateWindow).toHaveBeenCalledTimes(1)
    expect(onFocusTerminal).toHaveBeenCalled()
  })

  it('closes the active window with Ctrl+W and prevents the browser shortcut', () => {
    const onCloseWindow = vi.fn()
    const onFocusTerminal = vi.fn()

    render(
      <TmuxTerminalPanel
        {...baseProps}
        activeWindowIndex={2}
        onCloseWindow={onCloseWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    expect(fireEvent.keyDown(document, { key: 'w', ctrlKey: true })).toBe(false)
    expect(onCloseWindow).toHaveBeenCalledWith(2)
    expect(onFocusTerminal).toHaveBeenCalled()
  })

  it('selects the adjacent tmux windows with Ctrl+PageUp and Ctrl+PageDown', () => {
    const onSelectWindow = vi.fn()
    const onFocusTerminal = vi.fn()
    const windows = [
      {
        session: 'dev',
        index: 1,
        name: 'left',
        displayName: 'left',
        active: false,
        panes: 1,
      },
      {
        session: 'dev',
        index: 5,
        name: 'current',
        displayName: 'current',
        active: true,
        panes: 1,
      },
      {
        session: 'dev',
        index: 8,
        name: 'right',
        displayName: 'right',
        active: false,
        panes: 1,
      },
    ]

    const { rerender } = render(
      <TmuxTerminalPanel
        {...baseProps}
        windows={windows}
        activeWindowIndex={5}
        onSelectWindow={onSelectWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    expect(fireEvent.keyDown(document, { key: 'PageUp', ctrlKey: true })).toBe(false)
    expect(onSelectWindow).toHaveBeenLastCalledWith(1)

    rerender(
      <TmuxTerminalPanel
        {...baseProps}
        windows={windows}
        activeWindowIndex={5}
        onSelectWindow={onSelectWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    expect(fireEvent.keyDown(document, { key: 'PageDown', ctrlKey: true })).toBe(false)
    expect(onSelectWindow).toHaveBeenLastCalledWith(8)
    expect(onFocusTerminal).toHaveBeenCalled()
  })

  it('moves the active window with Ctrl+Shift+PageUp and Ctrl+Shift+PageDown', () => {
    const onReorderWindow = vi.fn()
    const onSelectWindow = vi.fn()
    const onFocusTerminal = vi.fn()
    const windows = [
      {
        session: 'dev',
        index: 1,
        name: 'left',
        displayName: 'left',
        active: false,
        panes: 1,
        tmuxWindowId: '@1',
      },
      {
        session: 'dev',
        index: 5,
        name: 'current',
        displayName: 'current',
        active: true,
        panes: 1,
        tmuxWindowId: '@5',
      },
      {
        session: 'dev',
        index: 8,
        name: 'right',
        displayName: 'right',
        active: false,
        panes: 1,
        tmuxWindowId: '@8',
      },
    ]

    render(
      <TmuxTerminalPanel
        {...baseProps}
        windows={windows}
        activeWindowIndex={5}
        onReorderWindow={onReorderWindow}
        onSelectWindow={onSelectWindow}
        onFocusTerminal={onFocusTerminal}
      />,
    )

    expect(
      fireEvent.keyDown(document, {
        key: 'PageUp',
        ctrlKey: true,
        shiftKey: true,
      }),
    ).toBe(false)
    expect(onReorderWindow).toHaveBeenLastCalledWith('@5', '@1')

    expect(
      fireEvent.keyDown(document, {
        key: 'PageDown',
        ctrlKey: true,
        shiftKey: true,
      }),
    ).toBe(false)
    expect(onReorderWindow).toHaveBeenLastCalledWith('@5', '@8')
    expect(onSelectWindow).not.toHaveBeenCalled()
    expect(onFocusTerminal).toHaveBeenCalled()
  })

  it('does not steal Ctrl+W from inputs outside the terminal panel', () => {
    const onCloseWindow = vi.fn()
    const input = document.createElement('input')
    document.body.append(input)

    render(<TmuxTerminalPanel {...baseProps} activeWindowIndex={2} onCloseWindow={onCloseWindow} />)

    expect(fireEvent.keyDown(input, { key: 'w', ctrlKey: true })).toBe(true)
    expect(onCloseWindow).not.toHaveBeenCalled()

    input.remove()
  })
})
