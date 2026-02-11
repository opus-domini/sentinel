import { useCallback, useEffect, useRef, useState } from 'react'
import { Keyboard } from 'lucide-react'
import NumPad from './NumPad'
import { hapticFeedback } from '@/lib/device'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ModifierState = 'off' | 'sticky' | 'locked'

type CsiDef = { type: 'letter'; letter: string } | { type: 'tilde'; n: number }

type TerminalControlsProps = {
  onSendKey: (key: string) => void
  onFlushComposition?: () => void
  onRefocus: () => void
  disabled?: boolean
  isKeyboardVisible?: () => boolean
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const LONG_PRESS_DELAY = 400
const REPEAT_INTERVAL = 80

const CSI_UP: CsiDef = { type: 'letter', letter: 'A' }
const CSI_DOWN: CsiDef = { type: 'letter', letter: 'B' }
const CSI_RIGHT: CsiDef = { type: 'letter', letter: 'C' }
const CSI_LEFT: CsiDef = { type: 'letter', letter: 'D' }
const CSI_HOME: CsiDef = { type: 'letter', letter: 'H' }
const CSI_END: CsiDef = { type: 'letter', letter: 'F' }
const CSI_PGUP: CsiDef = { type: 'tilde', n: 5 }
const CSI_PGDN: CsiDef = { type: 'tilde', n: 6 }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function ctrlCode(ch: string): string | null {
  const c = ch.toUpperCase().charCodeAt(0)
  if (c >= 0x41 && c <= 0x5a) return String.fromCharCode(c - 64)
  switch (ch) {
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

function buildCsi(def: CsiDef, ctrl: boolean, alt: boolean): string {
  const mod = 1 + (alt ? 2 : 0) + (ctrl ? 4 : 0)
  if (def.type === 'letter') {
    return mod === 1 ? `\x1b[${def.letter}` : `\x1b[1;${mod}${def.letter}`
  }
  return mod === 1 ? `\x1b[${def.n}~` : `\x1b[${def.n};${mod}~`
}

// ---------------------------------------------------------------------------
// ExtraKey — a single key with optional auto-repeat on long-press
// ---------------------------------------------------------------------------

type ExtraKeyProps = {
  label: React.ReactNode
  ariaLabel: string
  sequence?: string
  csi?: CsiDef
  repeat?: boolean
  disabled?: boolean
  ctrlRef: React.RefObject<ModifierState>
  altRef: React.RefObject<ModifierState>
  onSend: (seq: string) => void
  onFlushComposition?: () => void
  onConsume: () => void
  onRefocus: () => void
}

function ExtraKey({
  label,
  ariaLabel,
  sequence,
  csi,
  repeat,
  disabled,
  ctrlRef,
  altRef,
  onSend,
  onFlushComposition,
  onConsume,
  onRefocus,
}: ExtraKeyProps) {
  const longTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const repTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const lastTouch = useRef(0)

  const fire = useCallback(() => {
    // Flush any pending IME composition so the composed text is sent
    // to the PTY before this key sequence.
    onFlushComposition?.()

    const ctrl = ctrlRef.current !== 'off'
    const alt = altRef.current !== 'off'

    let seq: string
    if (csi) {
      seq = buildCsi(csi, ctrl, alt)
    } else {
      seq = sequence ?? ''
      if (seq.length === 1) {
        if (ctrl) {
          const mapped = ctrlCode(seq)
          if (mapped) seq = mapped
        }
        if (alt) seq = '\x1b' + seq
      }
    }

    if (seq) {
      onSend(seq)
      onConsume()
    }
  }, [csi, sequence, ctrlRef, altRef, onSend, onFlushComposition, onConsume])

  const clearTimers = useCallback(() => {
    if (longTimer.current !== null) {
      clearTimeout(longTimer.current)
      longTimer.current = null
    }
    if (repTimer.current !== null) {
      clearInterval(repTimer.current)
      repTimer.current = null
    }
  }, [])

  const handleTouchStart = useCallback(() => {
    lastTouch.current = Date.now()
    hapticFeedback()
    fire()
    if (repeat) {
      longTimer.current = setTimeout(() => {
        repTimer.current = setInterval(fire, REPEAT_INTERVAL)
      }, LONG_PRESS_DELAY)
    }
  }, [fire, repeat, clearTimers])

  const handleTouchEnd = useCallback(() => {
    clearTimers()
    onRefocus()
  }, [clearTimers, onRefocus])

  const handleClick = useCallback(() => {
    if (Date.now() - lastTouch.current < 700) return
    hapticFeedback()
    fire()
    onRefocus()
  }, [fire, onRefocus])

  useEffect(() => clearTimers, [clearTimers])

  return (
    <button
      type="button"
      className="terminal-key flex min-h-[32px] flex-1 items-center justify-center text-[10px] font-medium text-[#b0b0b0] active:bg-white/[0.08]"
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
      onTouchCancel={handleTouchEnd}
      onClick={handleClick}
      disabled={disabled}
      aria-label={ariaLabel}
    >
      {label}
    </button>
  )
}

// ---------------------------------------------------------------------------
// ModifierKey — Ctrl / Alt with sticky (tap) and locked (long-press)
// ---------------------------------------------------------------------------

type ModifierKeyProps = {
  label: string
  ariaLabel: string
  state: ModifierState
  disabled?: boolean
  onTap: () => void
  onLongPress: () => void
  onRefocus: () => void
}

function ModifierKey({
  label,
  ariaLabel,
  state,
  disabled,
  onTap,
  onLongPress,
  onRefocus,
}: ModifierKeyProps) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wasLong = useRef(false)
  const lastTouch = useRef(0)

  const handleTouchStart = useCallback(() => {
    lastTouch.current = Date.now()
    wasLong.current = false
    hapticFeedback()
    timer.current = setTimeout(() => {
      wasLong.current = true
      hapticFeedback()
      onLongPress()
    }, LONG_PRESS_DELAY)
  }, [onLongPress])

  const handleTouchEnd = useCallback(() => {
    if (timer.current !== null) {
      clearTimeout(timer.current)
      timer.current = null
    }
    if (!wasLong.current) onTap()
    onRefocus()
  }, [onTap, onRefocus])

  const handleClick = useCallback(() => {
    if (Date.now() - lastTouch.current < 700) return
    onTap()
    onRefocus()
  }, [onTap, onRefocus])

  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current)
    },
    [],
  )

  const bg =
    state === 'locked'
      ? 'bg-primary text-primary-foreground'
      : state === 'sticky'
        ? 'bg-primary/20 text-primary-text border border-primary/40'
        : 'text-[#b0b0b0] active:bg-white/[0.08]'

  return (
    <button
      type="button"
      className={`terminal-key flex min-h-[32px] flex-1 items-center justify-center rounded-sm text-[10px] font-semibold ${bg}`}
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
      onTouchCancel={handleTouchEnd}
      onClick={handleClick}
      disabled={disabled}
      aria-label={ariaLabel}
      aria-pressed={state !== 'off'}
    >
      {label}
    </button>
  )
}

// ---------------------------------------------------------------------------
// TerminalControls — inline two-row extra-keys bar
// ---------------------------------------------------------------------------

export default function TerminalControls({
  onSendKey,
  onFlushComposition,
  onRefocus,
  disabled,
  isKeyboardVisible,
}: TerminalControlsProps) {
  // -- Modifier state (ref + state kept in sync) --
  const [ctrlState, setCtrlState] = useState<ModifierState>('off')
  const [altState, setAltState] = useState<ModifierState>('off')
  const ctrlRef = useRef<ModifierState>('off')
  const altRef = useRef<ModifierState>('off')
  const controlsRef = useRef<HTMLDivElement>(null)

  const setCtrl = useCallback((s: ModifierState) => {
    ctrlRef.current = s
    setCtrlState(s)
  }, [])
  const setAlt = useCallback((s: ModifierState) => {
    altRef.current = s
    setAltState(s)
  }, [])

  const consumeModifiers = useCallback(() => {
    if (ctrlRef.current === 'sticky') setCtrl('off')
    if (altRef.current === 'sticky') setAlt('off')
  }, [setCtrl, setAlt])

  const ctrlTap = useCallback(
    () => setCtrl(ctrlRef.current === 'off' ? 'sticky' : 'off'),
    [setCtrl],
  )
  const ctrlLock = useCallback(() => setCtrl('locked'), [setCtrl])
  const altTap = useCallback(
    () => setAlt(altRef.current === 'off' ? 'sticky' : 'off'),
    [setAlt],
  )
  const altLock = useCallback(() => setAlt('locked'), [setAlt])

  // -- Prevent focus steal so tapping keys doesn't blur the terminal --
  useEffect(() => {
    const el = controlsRef.current
    if (!el || disabled) return
    const prevent = (e: Event) => {
      if (el.contains(e.target as Node)) e.preventDefault()
    }
    const opts: AddEventListenerOptions = { passive: false, capture: true }
    el.addEventListener('touchstart', prevent, opts)
    el.addEventListener('pointerdown', prevent, opts)
    el.addEventListener('mousedown', prevent, opts)
    return () => {
      el.removeEventListener('touchstart', prevent, { capture: true })
      el.removeEventListener('pointerdown', prevent, { capture: true })
      el.removeEventListener('mousedown', prevent, { capture: true })
    }
  }, [disabled])

  // -- Global key interception for Ctrl / Alt modifiers --
  useEffect(() => {
    const consume = (ch: string): boolean => {
      const ctrl = ctrlRef.current !== 'off'
      const alt = altRef.current !== 'off'
      if (!ctrl && !alt) return false

      let seq: string
      if (ctrl) {
        const mapped = ctrlCode(ch)
        seq = mapped ?? ch
        if (alt) seq = '\x1b' + seq
      } else {
        seq = '\x1b' + ch
      }

      onSendKey(seq)
      consumeModifiers()
      return true
    }

    const onKd = (e: KeyboardEvent) => {
      if (e.key.length === 1 && consume(e.key)) {
        e.preventDefault()
        e.stopPropagation()
      }
    }
    const onBi = (e: InputEvent) => {
      if (e.data?.length === 1 && consume(e.data)) {
        e.preventDefault()
        e.stopPropagation()
      }
    }
    const onIn = (e: Event) => {
      const ie = e as InputEvent
      if (ie.data?.length === 1 && consume(ie.data)) {
        ie.preventDefault()
        ie.stopPropagation()
      }
    }

    document.addEventListener('keydown', onKd, { capture: true })
    document.addEventListener('beforeinput', onBi, { capture: true })
    document.addEventListener('input', onIn, { capture: true })
    return () => {
      document.removeEventListener('keydown', onKd, { capture: true })
      document.removeEventListener('beforeinput', onBi, { capture: true })
      document.removeEventListener('input', onIn, { capture: true })
    }
  }, [onSendKey, consumeModifiers])

  // -- Keyboard toggle (focus/blur terminal) --
  const lastToggleTouch = useRef(0)
  const toggleKb = useCallback(() => {
    const el = document.activeElement as HTMLElement | null
    if (
      el &&
      (el.tagName === 'TEXTAREA' ||
        el.tagName === 'INPUT' ||
        el.isContentEditable)
    ) {
      el.blur()
    } else {
      onRefocus()
    }
  }, [onRefocus])

  // Shared props for all ExtraKey instances
  const k = {
    disabled,
    ctrlRef,
    altRef,
    onSend: onSendKey,
    onFlushComposition,
    onConsume: consumeModifiers,
    onRefocus,
  } as const

  return (
    <div
      ref={controlsRef}
      className="flex flex-col border-y border-border bg-[#1c1c1e]"
    >
      {/* Row 1: modifiers + navigation */}
      <div className="flex">
        <ExtraKey label="ESC" ariaLabel="Escape" sequence={'\x1b'} {...k} />
        <ExtraKey label="TAB" ariaLabel="Tab" sequence={'\t'} {...k} />
        <ModifierKey
          label="CTRL"
          ariaLabel="Ctrl modifier — tap: sticky, hold: lock"
          state={ctrlState}
          disabled={disabled}
          onTap={ctrlTap}
          onLongPress={ctrlLock}
          onRefocus={onRefocus}
        />
        <ModifierKey
          label="ALT"
          ariaLabel="Alt modifier — tap: sticky, hold: lock"
          state={altState}
          disabled={disabled}
          onTap={altTap}
          onLongPress={altLock}
          onRefocus={onRefocus}
        />
        <ExtraKey
          label="←"
          ariaLabel="Arrow left"
          csi={CSI_LEFT}
          repeat
          {...k}
        />
        <ExtraKey
          label="↓"
          ariaLabel="Arrow down"
          csi={CSI_DOWN}
          repeat
          {...k}
        />
        <ExtraKey label="↑" ariaLabel="Arrow up" csi={CSI_UP} repeat {...k} />
        <ExtraKey
          label="→"
          ariaLabel="Arrow right"
          csi={CSI_RIGHT}
          repeat
          {...k}
        />
      </div>

      {/* Row 2: symbols + extended navigation */}
      <div className="flex border-t border-white/[0.04]">
        <ExtraKey label="/" ariaLabel="Slash" sequence="/" repeat {...k} />
        <ExtraKey label="-" ariaLabel="Hyphen" sequence="-" repeat {...k} />
        <ExtraKey label="HOME" ariaLabel="Home" csi={CSI_HOME} {...k} />
        <ExtraKey label="END" ariaLabel="End" csi={CSI_END} {...k} />
        <ExtraKey label="PG↑" ariaLabel="Page up" csi={CSI_PGUP} {...k} />
        <ExtraKey label="PG↓" ariaLabel="Page down" csi={CSI_PGDN} {...k} />
        <NumPad
          onSendKey={onSendKey}
          disabled={disabled}
          onRefocus={onRefocus}
          isKeyboardVisible={isKeyboardVisible}
          triggerClassName="terminal-key-gesture flex min-h-[32px] flex-1 items-center justify-center text-[10px] font-medium text-[#b0b0b0] active:bg-white/[0.08]"
        />
        <button
          type="button"
          className="terminal-key flex min-h-[32px] flex-1 items-center justify-center text-[#b0b0b0] active:bg-white/[0.08]"
          onTouchStart={() => {
            lastToggleTouch.current = Date.now()
            hapticFeedback()
            toggleKb()
          }}
          onClick={() => {
            if (Date.now() - lastToggleTouch.current < 700) return
            toggleKb()
          }}
          disabled={disabled}
          aria-label="Toggle keyboard"
        >
          <Keyboard className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  )
}
