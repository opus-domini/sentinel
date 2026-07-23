import TerminalKeyButton from './TerminalKeyButton'
import type { ModifierName, TerminalInput, TerminalModifiers } from '@/lib/terminalInput'

type TerminalAdvancedKeysProps = {
  inputEnabled: boolean
  onSendKey: (input: TerminalInput) => boolean
  onRefocus: () => void
  modifiers: TerminalModifiers
  onToggleModifier: (modifier: ModifierName) => void
  onLockModifier: (modifier: ModifierName) => void
}

const ARROW_KEYS: Array<{
  label: string
  ariaLabel: string
  letter: string
}> = [
  { label: '←', ariaLabel: 'Arrow left', letter: 'D' },
  { label: '↓', ariaLabel: 'Arrow down', letter: 'B' },
  { label: '↑', ariaLabel: 'Arrow up', letter: 'A' },
  { label: '→', ariaLabel: 'Arrow right', letter: 'C' },
]

const NAVIGATION_KEYS: Array<{
  label: string
  ariaLabel: string
  input: TerminalInput
}> = [
  { label: 'HOME', ariaLabel: 'Home', input: { csi: { type: 'letter', letter: 'H' } } },
  { label: 'END', ariaLabel: 'End', input: { csi: { type: 'letter', letter: 'F' } } },
  { label: 'PG↑', ariaLabel: 'Page up', input: { csi: { type: 'tilde', number: 5 } } },
  { label: 'PG↓', ariaLabel: 'Page down', input: { csi: { type: 'tilde', number: 6 } } },
  { label: '/', ariaLabel: 'Slash', input: '/' },
  { label: '-', ariaLabel: 'Hyphen', input: '-' },
]

export default function TerminalAdvancedKeys({
  inputEnabled,
  onSendKey,
  onRefocus,
  modifiers,
  onToggleModifier,
  onLockModifier,
}: TerminalAdvancedKeysProps) {
  return (
    <section
      aria-label="Advanced terminal keys"
      className="border-t border-x border-border bg-surface-terminal-bar p-2"
    >
      <p className="mb-1.5 px-0.5 text-[10px] font-semibold text-secondary-foreground">
        Navigation
      </p>
      <div className="grid grid-cols-5 gap-1.5">
        <TerminalKeyButton
          ariaLabel="SHIFT modifier — tap: sticky, hold: lock"
          onPress={() => onToggleModifier('shift')}
          onLongPress={() => onLockModifier('shift')}
          onRefocus={onRefocus}
          disabled={!inputEnabled}
          pressed={modifiers.shift !== 'off'}
          className="w-full border border-border-subtle bg-surface-raised"
        >
          SHIFT
        </TerminalKeyButton>
        {ARROW_KEYS.map((key) => (
          <TerminalKeyButton
            key={key.ariaLabel}
            ariaLabel={key.ariaLabel}
            onPress={() => onSendKey({ csi: { type: 'letter', letter: key.letter } })}
            onRefocus={onRefocus}
            disabled={!inputEnabled}
            repeat
            className="w-full border border-border-subtle bg-surface-raised"
          >
            {key.label}
          </TerminalKeyButton>
        ))}
      </div>
      <div className="mt-1.5 grid grid-cols-6 gap-1.5">
        {NAVIGATION_KEYS.map((key) => (
          <TerminalKeyButton
            key={key.ariaLabel}
            ariaLabel={key.ariaLabel}
            onPress={() => onSendKey(key.input)}
            onRefocus={onRefocus}
            disabled={!inputEnabled}
            className="w-full border border-border-subtle bg-surface-raised"
          >
            {key.label}
          </TerminalKeyButton>
        ))}
      </div>
    </section>
  )
}
