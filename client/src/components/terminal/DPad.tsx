import { useEffect, useRef, useState } from 'react'
import { Move } from 'lucide-react'
import { hapticFeedback } from '@/lib/device'

type DPadProps = {
  onSendKey: (key: string) => void
  disabled?: boolean
  onRefocus?: () => void
  isKeyboardVisible?: () => boolean
}

type Direction = 'up' | 'down' | 'left' | 'right' | null

const ARROW_KEYS: Record<string, string> = {
  up: '\x1b[A',
  down: '\x1b[B',
  right: '\x1b[C',
  left: '\x1b[D',
}

const LONG_PRESS_MS = 150
const JOYSTICK_RADIUS = 70
const DEAD_ZONE = 15
const REPEAT_INITIAL_MS = 250
const REPEAT_MIN_MS = 400
const REPEAT_MAX_MS = 1500

function getDirection(dx: number, dy: number): Direction {
  const dist = Math.sqrt(dx * dx + dy * dy)
  if (dist < DEAD_ZONE) return null
  const angle = Math.atan2(dy, dx)
  if (angle >= -Math.PI / 4 && angle < Math.PI / 4) return 'right'
  if (angle >= Math.PI / 4 && angle < (3 * Math.PI) / 4) return 'down'
  if (angle >= (-3 * Math.PI) / 4 && angle < -Math.PI / 4) return 'up'
  return 'left'
}

function repeatInterval(dx: number, dy: number): number {
  const dist = Math.sqrt(dx * dx + dy * dy)
  const ratio = Math.min(1, Math.max(0, (dist - DEAD_ZONE) / JOYSTICK_RADIUS))
  return REPEAT_MAX_MS - ratio * (REPEAT_MAX_MS - REPEAT_MIN_MS)
}

export default function DPad({
  onSendKey,
  disabled,
  onRefocus,
  isKeyboardVisible,
}: DPadProps) {
  const [active, setActive] = useState(false)
  const [center, setCenter] = useState({ x: 0, y: 0 })
  const [knob, setKnob] = useState({ x: 0, y: 0 })
  const [direction, setDirection] = useState<Direction>(null)

  const buttonRef = useRef<HTMLButtonElement>(null)
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const repeatTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const directionRef = useRef<Direction>(null)
  const knobRef = useRef({ x: 0, y: 0 })
  const centerRef = useRef({ x: 0, y: 0 })
  const kbVisibleRef = useRef(false)

  const onSendKeyRef = useRef(onSendKey)
  const onRefocusRef = useRef(onRefocus)
  const isKeyboardVisibleRef = useRef(isKeyboardVisible)
  onSendKeyRef.current = onSendKey
  onRefocusRef.current = onRefocus
  isKeyboardVisibleRef.current = isKeyboardVisible

  useEffect(() => {
    const button = buttonRef.current
    if (!button || disabled) return

    let gestureActive = false

    const clearAllTimers = () => {
      if (longPressTimer.current !== null) {
        clearTimeout(longPressTimer.current)
        longPressTimer.current = null
      }
      if (repeatTimer.current !== null) {
        clearTimeout(repeatTimer.current)
        repeatTimer.current = null
      }
    }

    const scheduleRepeat = () => {
      if (repeatTimer.current !== null) clearTimeout(repeatTimer.current)
      const dir = directionRef.current
      if (!dir) return
      const dx = knobRef.current.x - centerRef.current.x
      const dy = knobRef.current.y - centerRef.current.y
      const interval = repeatInterval(dx, dy)
      repeatTimer.current = setTimeout(() => {
        if (directionRef.current) {
          onSendKeyRef.current(ARROW_KEYS[directionRef.current])
          scheduleRepeat()
        }
      }, interval)
    }

    const onMove = (e: TouchEvent) => {
      if (!gestureActive) return
      e.preventDefault()
      const touch = e.touches[0]

      const cx = centerRef.current.x
      const cy = centerRef.current.y
      let dx = touch.clientX - cx
      let dy = touch.clientY - cy
      const dist = Math.sqrt(dx * dx + dy * dy)
      if (dist > JOYSTICK_RADIUS) {
        dx = (dx / dist) * JOYSTICK_RADIUS
        dy = (dy / dist) * JOYSTICK_RADIUS
      }
      const pos = { x: cx + dx, y: cy + dy }
      knobRef.current = pos
      setKnob(pos)

      const newDir = getDirection(dx, dy)
      if (newDir !== directionRef.current) {
        directionRef.current = newDir
        setDirection(newDir)
        if (newDir) {
          onSendKeyRef.current(ARROW_KEYS[newDir])
          hapticFeedback()
          if (repeatTimer.current !== null) clearTimeout(repeatTimer.current)
          repeatTimer.current = setTimeout(scheduleRepeat, REPEAT_INITIAL_MS)
        } else {
          if (repeatTimer.current !== null) {
            clearTimeout(repeatTimer.current)
            repeatTimer.current = null
          }
        }
      }
    }

    const onEnd = (e: TouchEvent) => {
      e.preventDefault()
      clearAllTimers()
      gestureActive = false
      directionRef.current = null
      setDirection(null)
      setActive(false)
      document.removeEventListener('touchmove', onMove)
      document.removeEventListener('touchend', onEnd)
      document.removeEventListener('touchcancel', onEnd)
      if (kbVisibleRef.current) onRefocusRef.current?.()
    }

    const onStart = (e: TouchEvent) => {
      e.preventDefault()
      const touch = e.touches[0]
      kbVisibleRef.current = isKeyboardVisibleRef.current?.() ?? false

      document.addEventListener('touchmove', onMove, { passive: false })
      document.addEventListener('touchend', onEnd, { passive: false })
      document.addEventListener('touchcancel', onEnd, { passive: false })

      longPressTimer.current = setTimeout(() => {
        gestureActive = true
        const cx = touch.clientX
        const cy = touch.clientY - 60
        centerRef.current = { x: cx, y: cy }
        knobRef.current = { x: touch.clientX, y: touch.clientY }
        setCenter({ x: cx, y: cy })
        setKnob({ x: touch.clientX, y: touch.clientY })
        setActive(true)
        hapticFeedback()
      }, LONG_PRESS_MS)
    }

    button.addEventListener('touchstart', onStart, { passive: false })

    return () => {
      button.removeEventListener('touchstart', onStart)
      document.removeEventListener('touchmove', onMove)
      document.removeEventListener('touchend', onEnd)
      document.removeEventListener('touchcancel', onEnd)
      clearAllTimers()
      setActive(false)
    }
  }, [disabled])

  const dirLabel =
    direction === 'up'
      ? '\u2191'
      : direction === 'down'
        ? '\u2193'
        : direction === 'left'
          ? '\u2190'
          : direction === 'right'
            ? '\u2192'
            : ''

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className="terminal-key-gesture flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground active:scale-95 active:bg-surface-active"
        disabled={disabled}
        aria-label="Arrow keys (long press)"
      >
        <Move className="h-4 w-4" />
      </button>

      {active && (
        <div className="fixed inset-0 z-50" aria-hidden="true">
          <div
            className="absolute rounded-full border border-white/20 backdrop-blur-md"
            style={{
              width: JOYSTICK_RADIUS * 2,
              height: JOYSTICK_RADIUS * 2,
              left: center.x - JOYSTICK_RADIUS,
              top: center.y - JOYSTICK_RADIUS,
              background: 'rgba(255,255,255,0.06)',
            }}
          />
          <div
            className="absolute rounded-full bg-primary/80"
            style={{
              width: 28,
              height: 28,
              left: knob.x - 14,
              top: knob.y - 14,
            }}
          />
          {direction && (
            <div
              className="absolute text-center text-xs font-medium text-primary-foreground"
              style={{
                left: center.x - 20,
                top: center.y - JOYSTICK_RADIUS - 28,
                width: 40,
              }}
            >
              {dirLabel}
            </div>
          )}
        </div>
      )}
    </>
  )
}
