export type ModifierState = 'off' | 'sticky' | 'locked'
export type ModifierName = 'ctrl' | 'alt' | 'shift'
export type TerminalModifiers = Record<ModifierName, ModifierState>
export type CsiDefinition = { type: 'letter'; letter: string } | { type: 'tilde'; number: number }
export type TerminalInput = string | { sequence: string } | { csi: CsiDefinition }

export const EMPTY_TERMINAL_MODIFIERS: TerminalModifiers = {
  ctrl: 'off',
  alt: 'off',
  shift: 'off',
}

function ctrlCode(character: string): string | null {
  const code = character.toUpperCase().charCodeAt(0)
  if (code >= 0x41 && code <= 0x5a) return String.fromCharCode(code - 64)
  switch (character) {
    case '@':
    case ' ':
      return '\x00'
    case '[':
      return '\x1b'
    case '\\':
      return '\x1c'
    case ']':
      return '\x1d'
    case '^':
      return '\x1e'
    case '_':
      return '\x1f'
    case '?':
      return '\x7f'
    default:
      return null
  }
}

function activeModifiers(modifiers: TerminalModifiers) {
  return {
    ctrl: modifiers.ctrl !== 'off',
    alt: modifiers.alt !== 'off',
    shift: modifiers.shift !== 'off',
  }
}

function buildCsi(definition: CsiDefinition, modifiers: TerminalModifiers): string {
  const active = activeModifiers(modifiers)
  const parameter = 1 + (active.shift ? 1 : 0) + (active.alt ? 2 : 0) + (active.ctrl ? 4 : 0)
  if (definition.type === 'letter') {
    return parameter === 1 ? `\x1b[${definition.letter}` : `\x1b[1;${parameter}${definition.letter}`
  }
  return parameter === 1 ? `\x1b[${definition.number}~` : `\x1b[${definition.number};${parameter}~`
}

function applySequenceModifiers(sequence: string, modifiers: TerminalModifiers): string {
  const active = activeModifiers(modifiers)
  if (sequence === '\t' && active.shift) {
    const reverseTab = '\x1b[Z'
    return active.alt ? `\x1b${reverseTab}` : reverseTab
  }

  if (sequence.length !== 1) {
    return sequence
  }

  let next = sequence
  if (active.shift && /^[a-z]$/.test(next)) {
    next = next.toUpperCase()
  }
  if (active.ctrl) {
    next = ctrlCode(next) ?? next
  }
  if (active.alt) {
    next = `\x1b${next}`
  }
  return next
}

export function transformTerminalInput(
  input: TerminalInput,
  modifiers: TerminalModifiers,
): { data: string; consumesSticky: boolean } {
  if (typeof input === 'object' && 'csi' in input) {
    return {
      data: buildCsi(input.csi, modifiers),
      consumesSticky: true,
    }
  }

  const sequence = typeof input === 'string' ? input : input.sequence
  const eligible = sequence.length === 1
  return {
    data: applySequenceModifiers(sequence, modifiers),
    consumesSticky: eligible,
  }
}

export function consumeStickyModifiers(modifiers: TerminalModifiers): TerminalModifiers {
  return {
    ctrl: modifiers.ctrl === 'sticky' ? 'off' : modifiers.ctrl,
    alt: modifiers.alt === 'sticky' ? 'off' : modifiers.alt,
    shift: modifiers.shift === 'sticky' ? 'off' : modifiers.shift,
  }
}

export function toggleStickyModifier(
  modifiers: TerminalModifiers,
  modifier: ModifierName,
): TerminalModifiers {
  return {
    ...modifiers,
    [modifier]: modifiers[modifier] === 'off' ? 'sticky' : 'off',
  }
}

export function lockModifier(
  modifiers: TerminalModifiers,
  modifier: ModifierName,
): TerminalModifiers {
  return {
    ...modifiers,
    [modifier]: modifiers[modifier] === 'locked' ? 'off' : 'locked',
  }
}
