import { describe, expect, it } from 'vitest'
import {
  EMPTY_TERMINAL_MODIFIERS,
  consumeStickyModifiers,
  lockModifier,
  toggleStickyModifier,
  transformTerminalInput,
} from './terminalInput'

describe('terminalInput', () => {
  it('applies Ctrl, Alt and Shift to eligible text', () => {
    expect(
      transformTerminalInput('a', {
        ctrl: 'sticky',
        alt: 'locked',
        shift: 'sticky',
      }),
    ).toEqual({ data: '\x1b\x01', consumesSticky: true })
  })

  it('preserves composed and multibyte text without consuming modifiers', () => {
    expect(
      transformTerminalInput('ação', {
        ctrl: 'locked',
        alt: 'off',
        shift: 'off',
      }),
    ).toEqual({ data: 'ação', consumesSticky: false })
  })

  it('builds CSI sequences with xterm modifier parameters', () => {
    expect(
      transformTerminalInput(
        { csi: { type: 'letter', letter: 'A' } },
        { ctrl: 'sticky', alt: 'off', shift: 'sticky' },
      ),
    ).toEqual({ data: '\x1b[1;6A', consumesSticky: true })
    expect(
      transformTerminalInput({ csi: { type: 'tilde', number: 5 } }, EMPTY_TERMINAL_MODIFIERS),
    ).toEqual({ data: '\x1b[5~', consumesSticky: true })
  })

  it('maps Shift+Tab and preserves locked modifiers after consumption', () => {
    const modifiers = {
      ctrl: 'sticky' as const,
      alt: 'locked' as const,
      shift: 'sticky' as const,
    }

    expect(transformTerminalInput('\t', modifiers).data).toBe('\x1b\x1b[Z')
    expect(consumeStickyModifiers(modifiers)).toEqual({
      ctrl: 'off',
      alt: 'locked',
      shift: 'off',
    })
  })

  it('toggles sticky and locked states explicitly', () => {
    const sticky = toggleStickyModifier(EMPTY_TERMINAL_MODIFIERS, 'ctrl')
    expect(sticky.ctrl).toBe('sticky')
    expect(toggleStickyModifier(sticky, 'ctrl').ctrl).toBe('off')
    expect(lockModifier(sticky, 'ctrl').ctrl).toBe('locked')
    expect(lockModifier({ ...sticky, ctrl: 'locked' }, 'ctrl').ctrl).toBe('off')
  })
})
