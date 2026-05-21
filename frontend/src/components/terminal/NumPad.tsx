import { useEffect, useRef, useState } from 'react'
import { Hash } from 'lucide-react'
import { hapticFeedback } from '@/lib/device'

type NumPadProps = {
  onSendKey: (key: string) => void
  disabled?: boolean
  onRefocus?: () => void
  isKeyboardVisible?: () => boolean
  triggerClassName?: string
}

const LONG_PRESS_MS = 150
const CELL_W = 56
const CELL_H = 48
const GAP = 6
const PAD = 8

// Grid layout: 7 8 9 / 4 5 6 / 1 2 3 / (0)
const GRID: Array<Array<string>> = [
  ['7', '8', '9'],
  ['4', '5', '6'],
  ['1', '2', '3'],
  ['0'],
]

const GRID_COLS = 3
const GRID_ROWS = GRID.length
const GRID_W = PAD * 2 + GRID_COLS * CELL_W + (GRID_COLS - 1) * GAP
const GRID_H = PAD * 2 + GRID_ROWS * CELL_H + (GRID_ROWS - 1) * GAP

function getNumAtPoint(
  px: number,
  py: number,
  originX: number,
  originY: number,
): string | null {
  const lx = px - originX - PAD
  const ly = py - originY - PAD
  if (lx < 0 || ly < 0) return null

  const col = Math.floor(lx / (CELL_W + GAP))
  const row = Math.floor(ly / (CELL_H + GAP))

  if (row < 0 || row >= GRID_ROWS) return null
  const rowCells = GRID[row]
  if (col < 0 || col >= rowCells.length) return null

  const cellLeft = col * (CELL_W + GAP)
  const cellTop = row * (CELL_H + GAP)
  if (lx > cellLeft + CELL_W || ly > cellTop + CELL_H) return null

  return rowCells[col]
}

export default function NumPad({
  onSendKey,
  disabled,
  onRefocus,
  isKeyboardVisible,
  triggerClassName,
}: NumPadProps) {
  const [active, setActive] = useState(false)
  const [origin, setOrigin] = useState({ x: 0, y: 0 })
  const [highlighted, setHighlighted] = useState<string | null>(null)

  const buttonRef = useRef<HTMLButtonElement>(null)
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const highlightedRef = useRef<string | null>(null)
  const originRef = useRef({ x: 0, y: 0 })
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

    const onMove = (e: TouchEvent) => {
      if (!gestureActive) return
      e.preventDefault()
      const touch = e.touches[0]

      const num = getNumAtPoint(
        touch.clientX,
        touch.clientY,
        originRef.current.x,
        originRef.current.y,
      )
      if (num !== highlightedRef.current) {
        highlightedRef.current = num
        setHighlighted(num)
        if (num !== null) hapticFeedback()
      }
    }

    const onEnd = (e: TouchEvent) => {
      e.preventDefault()
      if (longPressTimer.current !== null) {
        clearTimeout(longPressTimer.current)
        longPressTimer.current = null
      }
      const num = highlightedRef.current
      if (gestureActive && num !== null) {
        onSendKeyRef.current(num)
      }
      gestureActive = false
      highlightedRef.current = null
      setHighlighted(null)
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
        let ox = touch.clientX - GRID_W / 2
        let oy = touch.clientY - GRID_H - 20
        ox = Math.max(4, Math.min(window.innerWidth - GRID_W - 4, ox))
        oy = Math.max(4, Math.min(window.innerHeight - GRID_H - 4, oy))
        originRef.current = { x: ox, y: oy }
        setOrigin({ x: ox, y: oy })
        setHighlighted(null)
        highlightedRef.current = null
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
      if (longPressTimer.current !== null) {
        clearTimeout(longPressTimer.current)
        longPressTimer.current = null
      }
      setActive(false)
    }
  }, [disabled])

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className={
          triggerClassName ??
          'terminal-key-gesture flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground active:scale-95 active:bg-surface-active'
        }
        disabled={disabled}
        aria-label="Number pad (long press)"
      >
        <Hash className="h-4 w-4" />
      </button>

      {active && (
        <div className="fixed inset-0 z-50" aria-hidden="true">
          <div
            className="absolute rounded-lg border border-white/20 bg-card/95 backdrop-blur-md"
            style={{
              left: origin.x,
              top: origin.y,
              width: GRID_W,
              padding: PAD,
            }}
          >
            {GRID.map((row, ri) => (
              <div
                key={ri}
                className="flex"
                style={{ gap: GAP, marginTop: ri > 0 ? GAP : 0 }}
              >
                {row.map((num) => (
                  <div
                    key={num}
                    className={`flex items-center justify-center rounded-md text-sm font-medium ${
                      highlighted === num
                        ? 'bg-primary text-primary-foreground'
                        : 'bg-surface-active text-foreground'
                    }`}
                    style={{
                      width: row.length === 1 ? GRID_W - PAD * 2 : CELL_W,
                      height: CELL_H,
                    }}
                  >
                    {num}
                  </div>
                ))}
              </div>
            ))}

            {highlighted !== null && (
              <div className="mt-1.5 text-center text-xs text-muted-foreground">
                Release to send &apos;{highlighted}&apos;
              </div>
            )}
          </div>
        </div>
      )}
    </>
  )
}
