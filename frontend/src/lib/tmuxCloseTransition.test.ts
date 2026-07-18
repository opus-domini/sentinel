import { describe, expect, it } from 'vitest'

import { deriveTmuxCloseTransition } from './tmuxCloseTransition'
import type { TmuxCloseTarget } from './tmuxCloseTransition'
import type { PaneInfo, WindowInfo } from '@/types'

function windowInfo(index: number, active = false): WindowInfo {
  return {
    session: 'dev',
    index,
    name: `window-${index}`,
    displayName: `window-${index}`,
    active,
    panes: 1,
  }
}

function paneInfo(
  windowIndex: number,
  paneIndex: number,
  paneId: string,
  active = false,
): PaneInfo {
  return {
    session: 'dev',
    windowIndex,
    paneIndex,
    paneId,
    title: paneId,
    active,
    tty: `/dev/pts/${paneIndex}`,
  }
}

type Scenario = {
  name: string
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
  target: TmuxCloseTarget
  wantWindow: number | null
  wantPane: string | null
  wantWindows: Array<number>
  sessionEnded?: boolean
}

const scenarios: Array<Scenario> = [
  {
    name: 'closing an inactive window preserves the active window and pane',
    windows: [windowInfo(1, true), windowInfo(2)],
    panes: [paneInfo(1, 0, '%1', true), paneInfo(2, 0, '%2')],
    target: { type: 'window', windowIndex: 2 },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing an active middle window focuses the next window',
    windows: [windowInfo(1), windowInfo(2, true), windowInfo(3)],
    panes: [paneInfo(1, 0, '%1'), paneInfo(2, 0, '%2', true), paneInfo(3, 0, '%3')],
    target: { type: 'window', windowIndex: 2 },
    wantWindow: 3,
    wantPane: '%3',
    wantWindows: [1, 3],
  },
  {
    name: 'closing the highest active window falls back to the previous window',
    windows: [windowInfo(1), windowInfo(2, true)],
    panes: [paneInfo(1, 0, '%1'), paneInfo(2, 0, '%2', true)],
    target: { type: 'window', windowIndex: 2 },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing an inactive pane preserves the active pane',
    windows: [windowInfo(1, true)],
    panes: [paneInfo(1, 0, '%1', true), paneInfo(1, 1, '%2')],
    target: { type: 'pane', paneID: '%2' },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing an active middle pane focuses the next pane',
    windows: [windowInfo(1, true)],
    panes: [paneInfo(1, 0, '%1'), paneInfo(1, 1, '%2', true), paneInfo(1, 2, '%3')],
    target: { type: 'pane', paneID: '%2' },
    wantWindow: 1,
    wantPane: '%3',
    wantWindows: [1],
  },
  {
    name: 'closing the highest active pane falls back to the previous pane',
    windows: [windowInfo(1, true)],
    panes: [paneInfo(1, 0, '%1'), paneInfo(1, 1, '%2', true)],
    target: { type: 'pane', paneID: '%2' },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing the last pane of an inactive window preserves focus',
    windows: [windowInfo(1, true), windowInfo(2)],
    panes: [paneInfo(1, 0, '%1', true), paneInfo(2, 0, '%2')],
    target: { type: 'pane', paneID: '%2' },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing the last pane of the active window focuses the next window',
    windows: [windowInfo(1, true), windowInfo(2)],
    panes: [paneInfo(1, 0, '%1', true), paneInfo(2, 0, '%2')],
    target: { type: 'pane', paneID: '%1' },
    wantWindow: 2,
    wantPane: '%2',
    wantWindows: [2],
  },
  {
    name: 'closing the last pane of the highest active window focuses the previous window',
    windows: [windowInfo(1), windowInfo(2, true)],
    panes: [paneInfo(1, 0, '%1'), paneInfo(2, 0, '%2', true)],
    target: { type: 'pane', paneID: '%2' },
    wantWindow: 1,
    wantPane: '%1',
    wantWindows: [1],
  },
  {
    name: 'closing the only pane in the only window ends the session',
    windows: [windowInfo(1, true)],
    panes: [paneInfo(1, 0, '%1', true)],
    target: { type: 'pane', paneID: '%1' },
    wantWindow: null,
    wantPane: null,
    wantWindows: [],
    sessionEnded: true,
  },
  {
    name: 'closing the only window ends the session',
    windows: [windowInfo(1, true)],
    panes: [paneInfo(1, 0, '%1', true)],
    target: { type: 'window', windowIndex: 1 },
    wantWindow: null,
    wantPane: null,
    wantWindows: [],
    sessionEnded: true,
  },
]

describe('deriveTmuxCloseTransition', () => {
  for (const scenario of scenarios) {
    it(scenario.name, () => {
      const transition = deriveTmuxCloseTransition(
        scenario.windows,
        scenario.panes,
        scenario.windows.find((item) => item.active)?.index ?? null,
        scenario.panes.find((item) => item.active)?.paneId ?? null,
        scenario.target,
      )

      expect(transition.activeWindowIndex).toBe(scenario.wantWindow)
      expect(transition.activePaneID).toBe(scenario.wantPane)
      expect(transition.windows.map((item) => item.index)).toEqual(scenario.wantWindows)
      expect(transition.windows.filter((item) => item.active).map((item) => item.index)).toEqual(
        scenario.wantWindow === null ? [] : [scenario.wantWindow],
      )
      expect(transition.panes.filter((item) => item.active).map((item) => item.paneId)).toEqual(
        scenario.wantPane === null ? [] : [scenario.wantPane],
      )
      expect(transition.sessionEnded).toBe(scenario.sessionEnded === true)
    })
  }
})
