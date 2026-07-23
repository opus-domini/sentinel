import type { Terminal } from '@xterm/xterm'

type TerminalCell = {
  column: number
  row: number
}

type SelectionTerminal = Pick<Terminal, 'cols' | 'rows' | 'buffer' | 'select' | 'scrollLines'>

type TouchTerminalSelectionOptions = {
  screen: HTMLElement
  terminal: SelectionTerminal
  onSelectionChange: (hasSelection: boolean) => void
}

export type TouchTerminalSelectionController = {
  dispose: () => void
}

const EDGE_SCROLL_INTERVAL_MS = 80
const MAX_EDGE_ZONE_PX = 36

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

export function clientPointToTerminalCell(
  terminal: SelectionTerminal,
  screenRect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>,
  clientX: number,
  clientY: number,
): TerminalCell {
  const cols = Math.max(1, terminal.cols)
  const rows = Math.max(1, terminal.rows)
  const viewportRow = clamp(
    Math.floor((clientY - screenRect.top) / Math.max(1, screenRect.height / rows)),
    0,
    rows - 1,
  )
  const maxBufferRow = Math.max(0, terminal.buffer.active.length - 1)

  return {
    column: clamp(
      Math.floor((clientX - screenRect.left) / Math.max(1, screenRect.width / cols)),
      0,
      cols - 1,
    ),
    row: clamp(terminal.buffer.active.viewportY + viewportRow, 0, maxBufferRow),
  }
}

export function normalizeTerminalSelection(
  start: TerminalCell,
  end: TerminalCell,
  cols: number,
): { start: TerminalCell; length: number } {
  const startOffset = start.row * cols + start.column
  const endOffset = end.row * cols + end.column
  const first = startOffset <= endOffset ? start : end
  const last = startOffset <= endOffset ? end : start
  return {
    start: first,
    length: (last.row - first.row) * cols - first.column + last.column + 1,
  }
}

export function attachTouchTerminalSelection({
  screen,
  terminal,
  onSelectionChange,
}: TouchTerminalSelectionOptions): TouchTerminalSelectionController {
  let activePointerID: number | null = null
  let startCell: TerminalCell | null = null
  let lastClientX = 0
  let lastClientY = 0
  let edgeDirection = 0
  let edgeTimer: number | null = null

  const stopEdgeScroll = () => {
    edgeDirection = 0
    if (edgeTimer !== null) {
      window.clearInterval(edgeTimer)
      edgeTimer = null
    }
  }

  const updateSelection = () => {
    if (!startCell) return
    const endCell = clientPointToTerminalCell(
      terminal,
      screen.getBoundingClientRect(),
      lastClientX,
      lastClientY,
    )
    const normalized = normalizeTerminalSelection(startCell, endCell, Math.max(1, terminal.cols))
    terminal.select(normalized.start.column, normalized.start.row, normalized.length)
    onSelectionChange(normalized.length > 0)
  }

  const updateEdgeScroll = () => {
    const rect = screen.getBoundingClientRect()
    const edgeZone = Math.min(MAX_EDGE_ZONE_PX, rect.height / 4)
    const nextDirection =
      lastClientY <= rect.top + edgeZone ? -1 : lastClientY >= rect.bottom - edgeZone ? 1 : 0
    if (nextDirection === edgeDirection) return
    stopEdgeScroll()
    edgeDirection = nextDirection
    if (edgeDirection === 0) return
    edgeTimer = window.setInterval(() => {
      terminal.scrollLines(edgeDirection)
      updateSelection()
    }, EDGE_SCROLL_INTERVAL_MS)
  }

  const preventSelectionGesture = (event: PointerEvent) => {
    event.preventDefault()
    event.stopPropagation()
  }

  const onPointerDown = (event: PointerEvent) => {
    if (event.button !== 0) return
    preventSelectionGesture(event)
    if (activePointerID !== null && activePointerID !== event.pointerId) {
      activePointerID = null
      startCell = null
      stopEdgeScroll()
      return
    }

    activePointerID = event.pointerId
    lastClientX = event.clientX
    lastClientY = event.clientY
    startCell = clientPointToTerminalCell(
      terminal,
      screen.getBoundingClientRect(),
      event.clientX,
      event.clientY,
    )
    try {
      screen.setPointerCapture?.(event.pointerId)
    } catch {
      // Synthetic events and interrupted native gestures may not own an
      // active pointer. Selection still works through the document event flow.
    }
    updateSelection()
  }

  const onPointerMove = (event: PointerEvent) => {
    if (activePointerID !== event.pointerId) return
    preventSelectionGesture(event)
    lastClientX = event.clientX
    lastClientY = event.clientY
    updateSelection()
    updateEdgeScroll()
  }

  const finishPointer = (event: PointerEvent, update: boolean) => {
    if (activePointerID !== event.pointerId) return
    preventSelectionGesture(event)
    if (update) {
      lastClientX = event.clientX
      lastClientY = event.clientY
      updateSelection()
    }
    activePointerID = null
    startCell = null
    stopEdgeScroll()
  }

  const onPointerUp = (event: PointerEvent) => finishPointer(event, true)
  const onPointerCancel = (event: PointerEvent) => finishPointer(event, false)

  screen.addEventListener('pointerdown', onPointerDown, true)
  screen.addEventListener('pointermove', onPointerMove, true)
  screen.addEventListener('pointerup', onPointerUp, true)
  screen.addEventListener('pointercancel', onPointerCancel, true)
  screen.classList.add('touch-terminal-selecting')

  return {
    dispose: () => {
      stopEdgeScroll()
      activePointerID = null
      startCell = null
      screen.classList.remove('touch-terminal-selecting')
      screen.removeEventListener('pointerdown', onPointerDown, true)
      screen.removeEventListener('pointermove', onPointerMove, true)
      screen.removeEventListener('pointerup', onPointerUp, true)
      screen.removeEventListener('pointercancel', onPointerCancel, true)
    },
  }
}
