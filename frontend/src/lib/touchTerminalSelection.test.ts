// @vitest-environment jsdom
import { fireEvent } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  attachTouchTerminalSelection,
  clientPointToTerminalCell,
  normalizeTerminalSelection,
} from './touchTerminalSelection'

function createTerminal() {
  const terminal = {
    cols: 10,
    rows: 5,
    buffer: {
      active: {
        viewportY: 20,
        length: 100,
      },
    },
    select: vi.fn(),
    scrollLines: vi.fn((lines: number) => {
      terminal.buffer.active.viewportY += lines
    }),
  }
  return terminal
}

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('touchTerminalSelection', () => {
  it('maps viewport coordinates to clamped absolute buffer cells', () => {
    const terminal = createTerminal()
    expect(
      clientPointToTerminalCell(
        terminal as never,
        { left: 10, top: 20, width: 100, height: 50 },
        45,
        44,
      ),
    ).toEqual({ column: 3, row: 22 })
    expect(
      clientPointToTerminalCell(
        terminal as never,
        { left: 10, top: 20, width: 100, height: 50 },
        -100,
        1_000,
      ),
    ).toEqual({ column: 0, row: 24 })
  })

  it('normalizes forward and reverse multi-line ranges', () => {
    expect(normalizeTerminalSelection({ column: 2, row: 4 }, { column: 5, row: 6 }, 10)).toEqual({
      start: { column: 2, row: 4 },
      length: 24,
    })
    expect(normalizeTerminalSelection({ column: 5, row: 6 }, { column: 2, row: 4 }, 10)).toEqual({
      start: { column: 2, row: 4 },
      length: 24,
    })
  })

  it('selects through drag and preserves the range after pointer up', () => {
    const screen = document.createElement('div')
    document.body.append(screen)
    vi.spyOn(screen, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 100,
      bottom: 50,
      width: 100,
      height: 50,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const terminal = createTerminal()
    const onSelectionChange = vi.fn()
    const controller = attachTouchTerminalSelection({
      screen,
      terminal: terminal as never,
      onSelectionChange,
    })

    fireEvent.pointerDown(screen, { pointerId: 1, button: 0, clientX: 15, clientY: 5 })
    fireEvent.pointerMove(screen, { pointerId: 1, clientX: 65, clientY: 25 })
    fireEvent.pointerUp(screen, { pointerId: 1, clientX: 65, clientY: 25 })

    expect(terminal.select).toHaveBeenLastCalledWith(1, 20, 26)
    expect(onSelectionChange).toHaveBeenLastCalledWith(true)
    expect(screen.classList.contains('touch-terminal-selecting')).toBe(true)

    controller.dispose()
    expect(screen.classList.contains('touch-terminal-selecting')).toBe(false)
  })

  it('auto-scrolls at the edge and cancels timers during cleanup', () => {
    vi.useFakeTimers()
    const screen = document.createElement('div')
    vi.spyOn(screen, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const terminal = createTerminal()
    const controller = attachTouchTerminalSelection({
      screen,
      terminal: terminal as never,
      onSelectionChange: () => undefined,
    })

    fireEvent.pointerDown(screen, { pointerId: 3, button: 0, clientX: 20, clientY: 50 })
    fireEvent.pointerMove(screen, { pointerId: 3, clientX: 20, clientY: 99 })
    vi.advanceTimersByTime(160)

    expect(terminal.scrollLines).toHaveBeenCalledWith(1)
    controller.dispose()
    const calls = terminal.scrollLines.mock.calls.length
    vi.advanceTimersByTime(160)
    expect(terminal.scrollLines).toHaveBeenCalledTimes(calls)
  })
})
