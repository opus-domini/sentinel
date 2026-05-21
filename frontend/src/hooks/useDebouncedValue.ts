import { useEffect, useState } from 'react'

const DEFAULT_DEBOUNCE_MS = 180

export function useDebouncedValue<T>(
  value: T,
  delayMs: number = DEFAULT_DEBOUNCE_MS,
): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    if (Object.is(value, debounced)) {
      return
    }

    if (typeof value === 'string' && value.trim() === '') {
      setDebounced(value)
      return
    }

    const timeoutID = setTimeout(() => {
      setDebounced(value)
    }, delayMs)

    return () => {
      clearTimeout(timeoutID)
    }
  }, [debounced, delayMs, value])

  return debounced
}
