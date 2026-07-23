// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PaneStrip from './PaneStrip'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { PaneInfo } from '@/types'

const viewport = vi.hoisted(() => ({ touchOptimized: false }))

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: viewport.touchOptimized,
    touchCapable: viewport.touchOptimized,
    touchOptimized: viewport.touchOptimized,
  }),
}))

describe('PaneStrip', () => {
  afterEach(() => {
    cleanup()
    viewport.touchOptimized = false
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

  it('keeps active pane text neutral and marks selection with border only', () => {
    render(
      <TooltipProvider>
        <PaneStrip
          hasActiveSession={true}
          inspectorLoading={false}
          inspectorError=""
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

    const paneButton = screen.getByRole('button', { name: 'Select pane shell' })
    const paneChip = paneButton.parentElement

    expect(paneChip?.className).toContain('border-primary/60')
    expect(paneChip?.className).toContain('text-secondary-foreground')
    expect(paneChip?.className).not.toContain('text-primary-text')
  })

  it('consolidates split actions into one touch-safe menu on touch devices', () => {
    viewport.touchOptimized = true
    const onSplitPaneVertical = vi.fn()
    const onSplitPaneHorizontal = vi.fn()
    render(
      <TooltipProvider>
        <PaneStrip
          hasActiveSession
          inspectorLoading={false}
          inspectorError=""
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
          onSplitPaneVertical={onSplitPaneVertical}
          onSplitPaneHorizontal={onSplitPaneHorizontal}
        />
      </TooltipProvider>,
    )

    expect(screen.queryByRole('button', { name: 'Split vertical' })).toBeNull()
    const actions = screen.getByRole('button', { name: 'Create pane' })
    expect(actions.getAttribute('data-size')).toBe('xs')
    expect(actions.className).toContain('h-5')
    expect(actions.querySelector('.lucide-plus')).not.toBeNull()
    fireEvent.pointerDown(actions, { button: 0, ctrlKey: false })
    expect(screen.getByRole('menuitem', { name: 'Split horizontal' })).not.toBeNull()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Split vertical' }))

    expect(onSplitPaneVertical).toHaveBeenCalledTimes(1)
    expect(onSplitPaneHorizontal).not.toHaveBeenCalled()
  })
})
