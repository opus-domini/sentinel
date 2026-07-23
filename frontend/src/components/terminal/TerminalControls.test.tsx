// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TerminalControls from './TerminalControls'
import { EMPTY_TERMINAL_MODIFIERS } from '@/lib/terminalInput'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

function renderControls(overrides = {}) {
  const props = {
    onSendKey: vi.fn(() => true),
    onRefocus: vi.fn(),
    inputEnabled: true,
    modifiers: EMPTY_TERMINAL_MODIFIERS,
    onToggleModifier: vi.fn(),
    onLockModifier: vi.fn(),
    selectionMode: false,
    hasSelection: false,
    onEnterSelectionMode: vi.fn(),
    onCopySelection: vi.fn().mockResolvedValue(true),
    onCancelSelection: vi.fn(),
    ...overrides,
  }
  render(<TerminalControls {...props} />)
  return props
}

describe('TerminalControls', () => {
  it('sends Tab and Enter through the shared input boundary', () => {
    const props = renderControls()

    fireEvent.click(screen.getByRole('button', { name: 'Tab' }))
    fireEvent.click(screen.getByRole('button', { name: 'Enter' }))

    expect(props.onSendKey).toHaveBeenNthCalledWith(1, '\t')
    expect(props.onSendKey).toHaveBeenNthCalledWith(2, '\r')
  })

  it('delegates sticky and locked modifier state to the terminal hook', () => {
    vi.useFakeTimers()
    const props = renderControls()
    const ctrl = screen.getByRole('button', { name: /Ctrl modifier/i })

    fireEvent.pointerDown(ctrl, { pointerId: 1, button: 0 })
    fireEvent.pointerUp(ctrl, { pointerId: 1 })
    expect(props.onToggleModifier).toHaveBeenCalledWith('ctrl')

    fireEvent.pointerDown(ctrl, { pointerId: 2, button: 0 })
    vi.advanceTimersByTime(400)
    fireEvent.pointerUp(ctrl, { pointerId: 2 })
    expect(props.onLockModifier).toHaveBeenCalledWith('ctrl')
    expect(props.onToggleModifier).toHaveBeenCalledTimes(1)
  })

  it('does not send or report successful input while disconnected', () => {
    const props = renderControls({ inputEnabled: false })
    const enter = screen.getByRole('button', { name: 'Enter' })

    expect(enter.hasAttribute('disabled')).toBe(true)
    fireEvent.click(enter)
    expect(props.onSendKey).not.toHaveBeenCalled()
    expect(enter.getAttribute('title')).toContain('reconnecting')
  })

  it('opens compact advanced terminal navigation', () => {
    const props = renderControls()

    expect(screen.queryByLabelText('Scrollable terminal keys')).toBeNull()
    expect(screen.getByLabelText('Terminal keys').className).toContain('grid-cols-8')
    expect(screen.queryByRole('button', { name: 'Arrow left' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'More terminal keys' }))
    expect(screen.getByRole('region', { name: 'Advanced terminal keys' })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Shift modifier/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Arrow left' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Home' }))

    expect(props.onSendKey).toHaveBeenCalledWith({
      csi: { type: 'letter', letter: 'H' },
    })
  })

  it('uses content-only active styling for every active terminal key', () => {
    renderControls({
      isKeyboardVisible: () => true,
      modifiers: { ...EMPTY_TERMINAL_MODIFIERS, ctrl: 'locked' },
    })
    const more = screen.getByRole('button', { name: 'More terminal keys' })
    const keyboard = screen.getByRole('button', { name: 'Toggle keyboard' })
    const ctrl = screen.getByRole('button', { name: /Ctrl modifier/i })

    fireEvent.click(more)

    for (const button of [ctrl, more, keyboard]) {
      expect(button.getAttribute('aria-pressed')).toBe('true')
      expect(button.className).toContain('text-activity')
      expect(button.className).not.toContain('bg-primary/20')
      expect(button.className).not.toContain('bg-primary')
      expect(button.className).not.toContain('ring-1')
    }
  })

  it('uses pointer capture lifecycle without firing an extra click', () => {
    const props = renderControls()
    const escape = screen.getByRole('button', { name: 'Escape' })

    fireEvent.pointerDown(escape, { pointerId: 7, button: 0 })
    fireEvent.pointerUp(escape, { pointerId: 7 })
    fireEvent.click(escape, { detail: 1 })

    expect(props.onSendKey).toHaveBeenCalledTimes(1)
    expect(props.onRefocus).toHaveBeenCalledTimes(1)
  })

  it('keeps selection actions available while terminal input is disconnected', () => {
    const props = renderControls({ inputEnabled: false })

    fireEvent.click(screen.getByRole('button', { name: 'Select terminal text' }))
    expect(props.onEnterSelectionMode).toHaveBeenCalledTimes(1)
  })

  it('replaces normal keys with contextual Cancel and Copy actions in selection mode', () => {
    const props = renderControls({ selectionMode: true, hasSelection: true })

    expect(screen.queryByRole('button', { name: 'Enter' })).toBeNull()
    expect(screen.getByText('Drag over terminal text')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(props.onCopySelection).toHaveBeenCalledTimes(1)
    expect(props.onCancelSelection).toHaveBeenCalledTimes(1)
  })
})
