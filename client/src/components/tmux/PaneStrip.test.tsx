// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PaneStrip from './PaneStrip'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { PaneInfo } from '@/types'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

describe('PaneStrip', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders malformed pane payloads without calling trim on undefined', () => {
    const panes: Array<PaneInfo> = [
      {
        session: 'dev',
        windowIndex: 0,
        paneIndex: 0,
        paneId: undefined as unknown as string,
        title: undefined as unknown as string,
        active: true,
        tty: '/dev/pts/1',
      },
    ]

    render(
      <TooltipProvider>
        <PaneStrip
          hasActiveSession={true}
          inspectorLoading={false}
          inspectorError=""
          panes={panes}
          activeWindowIndex={0}
          activePaneID={null}
          onSelectPane={vi.fn()}
          onClosePane={vi.fn()}
          onRenamePane={vi.fn()}
          onSplitPaneVertical={vi.fn()}
          onSplitPaneHorizontal={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.getByText('pane 0')).not.toBeNull()
  })

  it('prefers the last valid pane snapshot over transient loading and error states', () => {
    render(
      <TooltipProvider>
        <PaneStrip
          hasActiveSession={true}
          inspectorLoading={true}
          inspectorError="Failed to fetch"
          panes={[
            {
              session: 'dev',
              windowIndex: 0,
              paneIndex: 0,
              paneId: '%1',
              title: 'shell',
              active: true,
              tty: '/dev/pts/1',
            },
          ]}
          activeWindowIndex={0}
          activePaneID="%1"
          onSelectPane={vi.fn()}
          onClosePane={vi.fn()}
          onRenamePane={vi.fn()}
          onSplitPaneVertical={vi.fn()}
          onSplitPaneHorizontal={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.getByText('shell')).not.toBeNull()
    expect(screen.queryByText('Failed to fetch')).toBeNull()
    expect(screen.queryByText('Loading panes')).toBeNull()
  })
})
