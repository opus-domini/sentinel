// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
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
  default: () => <div data-testid="window-strip">Window Strip</div>,
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
    expect(container.querySelector('main')?.className).toContain(
      'grid-rows-[40px_1fr_28px]',
    )
    expect(
      screen.getByTestId('window-strip').parentElement?.className,
    ).toContain('items-center')
  })

  it('shows session tabs on desktop when the sidebar is collapsed', () => {
    const { container } = render(
      <TmuxTerminalPanel {...baseProps} sidebarCollapsed />,
    )

    expect(screen.getByText('Session Tabs')).toBeTruthy()
    expect(container.querySelector('main')?.className).toContain(
      'grid-rows-[40px_30px_1fr_28px]',
    )
  })

  it('keeps session tabs visible on mobile even when the sidebar is expanded', () => {
    useIsMobileLayoutMock.mockReturnValue(true)

    render(<TmuxTerminalPanel {...baseProps} sidebarCollapsed={false} />)

    expect(screen.getByText('Session Tabs')).toBeTruthy()
  })

  it('shows a loading overlay while tmux is still attaching', () => {
    render(
      <TmuxTerminalPanel
        {...baseProps}
        connectionState="connecting"
        statusDetail="opening dev"
      />,
    )

    expect(screen.getByText('Waiting for tmux server...')).toBeTruthy()
  })

  it('shows reconnect copy when recovering the terminal socket', () => {
    render(
      <TmuxTerminalPanel
        {...baseProps}
        connectionState="connecting"
        statusDetail="reconnecting in 2s"
      />,
    )

    expect(screen.getByText('Reconnecting to tmux...')).toBeTruthy()
  })

  it('hides the loading overlay once connected', () => {
    render(<TmuxTerminalPanel {...baseProps} />)

    expect(screen.queryByText('Waiting for tmux server...')).toBeNull()
    expect(screen.queryByText('Reconnecting to tmux...')).toBeNull()
  })
})
