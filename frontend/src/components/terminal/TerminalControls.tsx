import { useCallback, useRef, useState } from 'react'
import { CornerDownLeft, Keyboard } from 'lucide-react'
import TerminalAdvancedKeys from './TerminalAdvancedKeys'
import TerminalKeyButton from './TerminalKeyButton'
import type { ModifierName, TerminalInput, TerminalModifiers } from '@/lib/terminalInput'

type TerminalControlsProps = {
  onSendKey: (input: TerminalInput) => boolean
  onFlushComposition?: () => void
  onRefocus: () => void
  inputEnabled: boolean
  modifiers: TerminalModifiers
  onToggleModifier: (modifier: ModifierName) => void
  onLockModifier: (modifier: ModifierName) => void
  selectionMode: boolean
  hasSelection: boolean
  onEnterSelectionMode: () => void
  onCopySelection: () => Promise<boolean>
  onCancelSelection: () => void
  isKeyboardVisible?: () => boolean
}

export default function TerminalControls({
  onSendKey,
  onFlushComposition,
  onRefocus,
  inputEnabled,
  modifiers,
  onToggleModifier,
  onLockModifier,
  selectionMode,
  hasSelection,
  onEnterSelectionMode,
  onCopySelection,
  onCancelSelection,
  isKeyboardVisible,
}: TerminalControlsProps) {
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const controlsRef = useRef<HTMLDivElement>(null)

  const sendKey = useCallback(
    (input: TerminalInput) => {
      onFlushComposition?.()
      return onSendKey(input)
    },
    [onFlushComposition, onSendKey],
  )

  const toggleKeyboard = useCallback(() => {
    const active = document.activeElement as HTMLElement | null
    if (isKeyboardVisible?.() || active?.matches('textarea, input, [contenteditable="true"]')) {
      active?.blur()
      return
    }
    onRefocus()
  }, [isKeyboardVisible, onRefocus])

  const modifierKey = (modifier: ModifierName, label: string) => (
    <TerminalKeyButton
      ariaLabel={`${label} modifier — tap: sticky, hold: lock`}
      onPress={() => onToggleModifier(modifier)}
      onLongPress={() => onLockModifier(modifier)}
      onRefocus={onRefocus}
      disabled={!inputEnabled}
      pressed={modifiers[modifier] !== 'off'}
      className="min-w-0 w-full px-1"
    >
      {label}
    </TerminalKeyButton>
  )

  if (selectionMode) {
    return (
      <div
        ref={controlsRef}
        className="relative z-20 flex h-11 items-stretch border-y border-border bg-surface-terminal-bar"
        data-testid="terminal-selection-controls"
      >
        <button
          type="button"
          className="min-w-16 shrink-0 px-3 text-[11px] font-semibold text-secondary-foreground"
          onMouseDown={(event) => event.preventDefault()}
          onClick={onCancelSelection}
        >
          Cancel
        </button>
        <p className="flex min-w-0 flex-1 items-center justify-center px-2 text-center text-[10px] text-secondary-foreground">
          Drag over terminal text
        </p>
        <button
          type="button"
          className="min-w-16 shrink-0 px-3 text-[11px] font-semibold text-primary-text disabled:opacity-40"
          disabled={!hasSelection}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            void onCopySelection()
          }}
        >
          Copy
        </button>
      </div>
    )
  }

  return (
    <div
      ref={controlsRef}
      className="relative z-20 bg-surface-terminal-bar"
      data-testid="terminal-controls"
    >
      {advancedOpen && (
        <TerminalAdvancedKeys
          inputEnabled={inputEnabled}
          onSendKey={sendKey}
          onRefocus={onRefocus}
          modifiers={modifiers}
          onToggleModifier={onToggleModifier}
          onLockModifier={onLockModifier}
        />
      )}
      <div
        className="grid h-11 grid-cols-8 items-stretch border-y border-border"
        aria-label="Terminal keys"
      >
        <TerminalKeyButton
          ariaLabel="Escape"
          onPress={() => sendKey('\x1b')}
          onRefocus={onRefocus}
          disabled={!inputEnabled}
          className="min-w-0 w-full px-1"
        >
          ESC
        </TerminalKeyButton>
        {modifierKey('ctrl', 'CTRL')}
        {modifierKey('alt', 'ALT')}
        <TerminalKeyButton
          ariaLabel="Tab"
          onPress={() => sendKey('\t')}
          onRefocus={onRefocus}
          disabled={!inputEnabled}
          className="min-w-0 w-full border-l border-border-subtle px-1"
        >
          TAB
        </TerminalKeyButton>
        <TerminalKeyButton
          ariaLabel="More terminal keys"
          onPress={() => setAdvancedOpen((open) => !open)}
          pressed={advancedOpen}
          expanded={advancedOpen}
          className="min-w-0 w-full px-1 text-[9px]"
        >
          MORE
        </TerminalKeyButton>
        <TerminalKeyButton
          ariaLabel="Enter"
          onPress={() => sendKey('\r')}
          onRefocus={onRefocus}
          disabled={!inputEnabled}
          className="min-w-0 w-full border-l border-border-subtle px-1 text-primary-text"
        >
          <CornerDownLeft className="size-4" aria-hidden="true" />
        </TerminalKeyButton>
        <TerminalKeyButton
          ariaLabel="Select terminal text"
          onPress={() => {
            setAdvancedOpen(false)
            onEnterSelectionMode()
          }}
          className="min-w-0 w-full px-1 text-[9px]"
        >
          SELECT
        </TerminalKeyButton>
        <TerminalKeyButton
          ariaLabel="Toggle keyboard"
          onPress={toggleKeyboard}
          pressed={isKeyboardVisible?.() ?? false}
          className="min-w-0 w-full px-1"
        >
          <Keyboard className="size-4" aria-hidden="true" />
        </TerminalKeyButton>
      </div>
    </div>
  )
}
