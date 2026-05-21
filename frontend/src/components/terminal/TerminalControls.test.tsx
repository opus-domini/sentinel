// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import TerminalControls from './TerminalControls'

afterEach(() => {
  cleanup()
})

describe('TerminalControls', () => {
  it('sends a plain tab when shift is not active', () => {
    const onSendKey = vi.fn()

    render(
      <TerminalControls onSendKey={onSendKey} onRefocus={() => undefined} />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Tab' }))

    expect(onSendKey).toHaveBeenCalledWith('\t')
  })

  it('sends reverse tab and consumes sticky shift', () => {
    const onSendKey = vi.fn()

    render(
      <TerminalControls onSendKey={onSendKey} onRefocus={() => undefined} />,
    )

    const shiftButton = screen.getByRole('button', { name: /Shift modifier/i })
    const tabButton = screen.getByRole('button', { name: 'Tab' })

    fireEvent.click(shiftButton)
    expect(shiftButton.getAttribute('aria-pressed')).toBe('true')

    fireEvent.click(tabButton)

    expect(onSendKey).toHaveBeenCalledWith('\x1b[Z')
    expect(shiftButton.getAttribute('aria-pressed')).toBe('false')
  })

  it('applies shift to CSI keys from the helper', () => {
    const onSendKey = vi.fn()

    render(
      <TerminalControls onSendKey={onSendKey} onRefocus={() => undefined} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Shift modifier/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Arrow up' }))

    expect(onSendKey).toHaveBeenCalledWith('\x1b[1;2A')
  })
})
