// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TerminalAdvancedKeys from './TerminalAdvancedKeys'
import { EMPTY_TERMINAL_MODIFIERS } from '@/lib/terminalInput'

afterEach(cleanup)

describe('TerminalAdvancedKeys', () => {
  it('exposes terminal navigation and symbols without duplicating the mobile keyboard', () => {
    const onSendKey = vi.fn(() => true)
    const onToggleModifier = vi.fn()
    render(
      <TerminalAdvancedKeys
        inputEnabled
        onSendKey={onSendKey}
        onRefocus={() => undefined}
        modifiers={EMPTY_TERMINAL_MODIFIERS}
        onToggleModifier={onToggleModifier}
        onLockModifier={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: /Shift modifier/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Arrow left' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Page up' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Slash' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Number/ })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /Shift modifier/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Page down' }))
    expect(onToggleModifier).toHaveBeenCalledWith('shift')
    expect(onSendKey).toHaveBeenCalledWith({
      csi: { type: 'tilde', number: 6 },
    })
  })

  it('disables only PTY actions when input is unavailable', () => {
    render(
      <TerminalAdvancedKeys
        inputEnabled={false}
        onSendKey={() => false}
        onRefocus={() => undefined}
        modifiers={EMPTY_TERMINAL_MODIFIERS}
        onToggleModifier={() => undefined}
        onLockModifier={() => undefined}
      />,
    )

    expect(screen.getByRole('button', { name: /Shift modifier/i }).hasAttribute('disabled')).toBe(
      true,
    )
    expect(screen.getByRole('button', { name: 'Home' }).hasAttribute('disabled')).toBe(true)
    expect(screen.getByRole('button', { name: 'Hyphen' }).hasAttribute('disabled')).toBe(true)
  })

  it('keeps locked Shift content-only without an active surface', () => {
    render(
      <TerminalAdvancedKeys
        inputEnabled
        onSendKey={() => true}
        onRefocus={() => undefined}
        modifiers={{ ...EMPTY_TERMINAL_MODIFIERS, shift: 'locked' }}
        onToggleModifier={() => undefined}
        onLockModifier={() => undefined}
      />,
    )

    const shift = screen.getByRole('button', { name: /Shift modifier/i })
    expect(shift.getAttribute('aria-pressed')).toBe('true')
    expect(shift.className).toContain('text-activity')
    expect(shift.className).not.toContain('bg-primary')
    expect(shift.className).not.toContain('ring-1')
  })
})
