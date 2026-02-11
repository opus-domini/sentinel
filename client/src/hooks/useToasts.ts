import { useCallback, useEffect, useRef, useState } from 'react'

export type ToastLevel = 'success' | 'error' | 'info'

export type ToastMessage = {
  id: number
  level: ToastLevel
  title: string
  message: string
}

type EnqueueToast = {
  level: ToastLevel
  title: string
  message: string
  ttlMs?: number
}

const DEFAULT_TTL_MS = 3600

export function useToasts() {
  const [toasts, setToasts] = useState<Array<ToastMessage>>([])
  const nextIdRef = useRef(1)
  const timersRef = useRef(new Map<number, number>())

  const dismissToast = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
    const timerId = timersRef.current.get(id)
    if (timerId !== undefined) {
      window.clearTimeout(timerId)
      timersRef.current.delete(id)
    }
  }, [])

  const pushToast = useCallback(
    ({ level, title, message, ttlMs = DEFAULT_TTL_MS }: EnqueueToast) => {
      const id = nextIdRef.current
      nextIdRef.current += 1

      setToasts((current) => {
        const next = [...current, { id, level, title, message }]
        if (next.length <= 5) {
          return next
        }

        const dropped = next.slice(0, next.length - 5)
        for (const toast of dropped) {
          const timerId = timersRef.current.get(toast.id)
          if (timerId !== undefined) {
            window.clearTimeout(timerId)
            timersRef.current.delete(toast.id)
          }
        }
        return next.slice(-5)
      })

      const timerId = window.setTimeout(() => {
        dismissToast(id)
      }, ttlMs)
      timersRef.current.set(id, timerId)
    },
    [dismissToast],
  )

  useEffect(() => {
    return () => {
      for (const timerId of timersRef.current.values()) {
        window.clearTimeout(timerId)
      }
      timersRef.current.clear()
    }
  }, [])

  return {
    toasts,
    pushToast,
    dismissToast,
  }
}
