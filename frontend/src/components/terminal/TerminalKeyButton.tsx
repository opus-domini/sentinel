import { useCallback, useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { hapticFeedback } from '@/lib/device'
import { cn } from '@/lib/utils'

const LONG_PRESS_DELAY_MS = 400
const REPEAT_INTERVAL_MS = 80

type TerminalKeyButtonProps = {
  children: ReactNode
  ariaLabel: string
  onPress: () => boolean | void
  onRefocus?: () => void
  onLongPress?: () => void
  repeat?: boolean
  disabled?: boolean
  pressed?: boolean
  expanded?: boolean
  className?: string
}

export default function TerminalKeyButton({
  children,
  ariaLabel,
  onPress,
  onRefocus,
  onLongPress,
  repeat = false,
  disabled = false,
  pressed,
  expanded,
  className,
}: TerminalKeyButtonProps) {
  const activePointerRef = useRef<number | null>(null)
  const longPressTimerRef = useRef<number | null>(null)
  const repeatTimerRef = useRef<number | null>(null)
  const longPressedRef = useRef(false)

  const clearTimers = useCallback(() => {
    if (longPressTimerRef.current !== null) {
      window.clearTimeout(longPressTimerRef.current)
      longPressTimerRef.current = null
    }
    if (repeatTimerRef.current !== null) {
      window.clearInterval(repeatTimerRef.current)
      repeatTimerRef.current = null
    }
  }, [])

  const activate = useCallback(() => {
    if (disabled) return false
    const accepted = onPress()
    if (accepted === false) return false
    hapticFeedback()
    return true
  }, [disabled, onPress])

  const finishPointer = useCallback(
    (pointerID: number, canceled: boolean) => {
      if (activePointerRef.current !== pointerID) return
      clearTimers()
      activePointerRef.current = null
      if (!canceled && onLongPress && !longPressedRef.current) {
        activate()
      }
      onRefocus?.()
    },
    [activate, clearTimers, onLongPress, onRefocus],
  )

  useEffect(() => clearTimers, [clearTimers])

  return (
    <button
      type="button"
      className={cn(
        'terminal-key inline-flex h-11 min-w-11 shrink-0 items-center justify-center rounded-sm px-2 text-[10px] font-semibold text-terminal-key-text disabled:cursor-not-allowed disabled:opacity-40',
        pressed && 'text-activity',
        className,
      )}
      aria-label={ariaLabel}
      aria-pressed={pressed}
      aria-expanded={expanded}
      disabled={disabled}
      title={disabled ? 'Terminal input is unavailable while reconnecting' : undefined}
      onMouseDown={(event) => event.preventDefault()}
      onPointerDown={(event) => {
        if (disabled || event.button !== 0 || activePointerRef.current !== null) return
        event.preventDefault()
        activePointerRef.current = event.pointerId
        longPressedRef.current = false
        event.currentTarget.setPointerCapture?.(event.pointerId)

        if (onLongPress) {
          longPressTimerRef.current = window.setTimeout(() => {
            longPressedRef.current = true
            onLongPress()
            hapticFeedback()
          }, LONG_PRESS_DELAY_MS)
          return
        }

        if (!activate() || !repeat) return
        longPressTimerRef.current = window.setTimeout(() => {
          repeatTimerRef.current = window.setInterval(activate, REPEAT_INTERVAL_MS)
        }, LONG_PRESS_DELAY_MS)
      }}
      onPointerUp={(event) => finishPointer(event.pointerId, false)}
      onPointerCancel={(event) => finishPointer(event.pointerId, true)}
      onPointerLeave={(event) => {
        if (activePointerRef.current === event.pointerId) {
          finishPointer(event.pointerId, true)
        }
      }}
      onClick={(event) => {
        if (event.detail !== 0 || disabled) return
        activate()
        onRefocus?.()
      }}
    >
      {children}
    </button>
  )
}
