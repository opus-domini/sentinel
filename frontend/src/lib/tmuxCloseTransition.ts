import type { PaneInfo, WindowInfo } from '@/types'

export type TmuxCloseTarget =
  | { type: 'window'; windowIndex: number }
  | { type: 'pane'; paneID: string }

export type TmuxCloseTransition = {
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
  activeWindowIndex: number | null
  activePaneID: string | null
  removed: boolean
  removedWindowIndex: number | null
  removedPaneCount: number
  sessionEnded: boolean
}

function sortWindows(windows: Array<WindowInfo>): Array<WindowInfo> {
  return [...windows].sort((left, right) => left.index - right.index)
}

function sortPanes(panes: Array<PaneInfo>): Array<PaneInfo> {
  return [...panes].sort((left, right) => {
    if (left.windowIndex !== right.windowIndex) {
      return left.windowIndex - right.windowIndex
    }
    return left.paneIndex - right.paneIndex
  })
}

function nextWindowIndex(windows: Array<WindowInfo>, removedWindowIndex: number): number | null {
  const ordered = sortWindows(windows)
  return (
    ordered.find((windowInfo) => windowInfo.index > removedWindowIndex)?.index ??
    ordered.at(-1)?.index ??
    null
  )
}

function firstPaneID(panes: Array<PaneInfo>, windowIndex: number): string | null {
  const inWindow = sortPanes(panes).filter((paneInfo) => paneInfo.windowIndex === windowIndex)
  return inWindow.find((paneInfo) => paneInfo.active)?.paneId ?? inWindow[0]?.paneId ?? null
}

function nextPaneID(
  panes: Array<PaneInfo>,
  windowIndex: number,
  removedPaneIndex: number,
): string | null {
  const inWindow = sortPanes(panes).filter((paneInfo) => paneInfo.windowIndex === windowIndex)
  return (
    inWindow.find((paneInfo) => paneInfo.paneIndex > removedPaneIndex)?.paneId ??
    inWindow.at(-1)?.paneId ??
    null
  )
}

function projectSelection(
  windows: Array<WindowInfo>,
  panes: Array<PaneInfo>,
  activeWindowIndex: number | null,
  activePaneID: string | null,
): Pick<TmuxCloseTransition, 'windows' | 'panes'> {
  return {
    windows: windows.map((windowInfo) => ({
      ...windowInfo,
      active: windowInfo.index === activeWindowIndex,
      panes: panes.filter((paneInfo) => paneInfo.windowIndex === windowInfo.index).length,
    })),
    panes: panes.map((paneInfo) => ({
      ...paneInfo,
      active: paneInfo.paneId === activePaneID,
    })),
  }
}

export function deriveTmuxCloseTransition(
  windows: Array<WindowInfo>,
  panes: Array<PaneInfo>,
  activeWindowIndex: number | null,
  activePaneID: string | null,
  target: TmuxCloseTarget,
): TmuxCloseTransition {
  const resolvedActiveWindowIndex =
    activeWindowIndex ?? windows.find((windowInfo) => windowInfo.active)?.index ?? null
  const resolvedActivePaneID =
    activePaneID ?? panes.find((paneInfo) => paneInfo.active)?.paneId ?? null

  let remainingWindows = windows
  let remainingPanes = panes
  let removedWindowIndex: number | null = null
  let removedPaneCount = 0
  let removedPane: PaneInfo | null = null

  if (target.type === 'window') {
    const removedWindow = windows.find((windowInfo) => windowInfo.index === target.windowIndex)
    if (!removedWindow) {
      return {
        windows,
        panes,
        activeWindowIndex: resolvedActiveWindowIndex,
        activePaneID: resolvedActivePaneID,
        removed: false,
        removedWindowIndex: null,
        removedPaneCount: 0,
        sessionEnded: false,
      }
    }
    removedWindowIndex = removedWindow.index
    removedPaneCount = panes.filter(
      (paneInfo) => paneInfo.windowIndex === removedWindow.index,
    ).length
    remainingWindows = windows.filter((windowInfo) => windowInfo.index !== removedWindow.index)
    remainingPanes = panes.filter((paneInfo) => paneInfo.windowIndex !== removedWindow.index)
  } else {
    removedPane = panes.find((paneInfo) => paneInfo.paneId === target.paneID) ?? null
    if (!removedPane) {
      return {
        windows,
        panes,
        activeWindowIndex: resolvedActiveWindowIndex,
        activePaneID: resolvedActivePaneID,
        removed: false,
        removedWindowIndex: null,
        removedPaneCount: 0,
        sessionEnded: false,
      }
    }
    removedPaneCount = 1
    remainingPanes = panes.filter((paneInfo) => paneInfo.paneId !== removedPane?.paneId)
    const windowStillExists = remainingPanes.some(
      (paneInfo) => paneInfo.windowIndex === removedPane?.windowIndex,
    )
    if (!windowStillExists) {
      removedWindowIndex = removedPane.windowIndex
      remainingWindows = windows.filter(
        (windowInfo) => windowInfo.index !== removedPane?.windowIndex,
      )
    }
  }

  if (remainingWindows.length === 0) {
    return {
      windows: [],
      panes: [],
      activeWindowIndex: null,
      activePaneID: null,
      removed: true,
      removedWindowIndex,
      removedPaneCount,
      sessionEnded: true,
    }
  }

  let nextActiveWindowIndex = resolvedActiveWindowIndex
  if (
    nextActiveWindowIndex === null ||
    !remainingWindows.some((windowInfo) => windowInfo.index === nextActiveWindowIndex)
  ) {
    nextActiveWindowIndex =
      removedWindowIndex === null
        ? (sortWindows(remainingWindows).find((windowInfo) => windowInfo.active)?.index ??
          sortWindows(remainingWindows)[0]?.index ??
          null)
        : nextWindowIndex(remainingWindows, removedWindowIndex)
  }

  let nextActivePaneID = resolvedActivePaneID
  const activePaneStillExists = remainingPanes.some(
    (paneInfo) =>
      paneInfo.paneId === nextActivePaneID && paneInfo.windowIndex === nextActiveWindowIndex,
  )
  if (!activePaneStillExists) {
    const closedActivePaneIndex =
      removedPane !== null &&
      removedPane.paneId === resolvedActivePaneID &&
      removedWindowIndex === null &&
      nextActiveWindowIndex === removedPane.windowIndex
        ? removedPane.paneIndex
        : null
    nextActivePaneID =
      nextActiveWindowIndex === null
        ? null
        : closedActivePaneIndex !== null
          ? nextPaneID(remainingPanes, nextActiveWindowIndex, closedActivePaneIndex)
          : firstPaneID(remainingPanes, nextActiveWindowIndex)
  }

  const projected = projectSelection(
    remainingWindows,
    remainingPanes,
    nextActiveWindowIndex,
    nextActivePaneID,
  )
  return {
    ...projected,
    activeWindowIndex: nextActiveWindowIndex,
    activePaneID: nextActivePaneID,
    removed: true,
    removedWindowIndex,
    removedPaneCount,
    sessionEnded: false,
  }
}
