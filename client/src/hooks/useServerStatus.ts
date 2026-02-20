import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'

function isNetworkError(error: unknown): boolean {
  if (error instanceof TypeError) return true
  if (!(error instanceof Error)) return false
  if (error.name === 'TypeError') return true
  const msg = error.message.toLowerCase()
  return (
    msg.includes('failed to fetch') ||
    msg.includes('networkerror') ||
    msg.includes('load failed') ||
    msg.includes('network request failed')
  )
}

export function useServerStatus() {
  const queryClient = useQueryClient()
  const [offline, setOffline] = useState(false)
  const offlineRef = useRef(false)

  useEffect(() => {
    const cache = queryClient.getQueryCache()
    const unsubscribe = cache.subscribe((event) => {
      if (event.type !== 'updated') return

      if (event.action.type === 'error' && isNetworkError(event.action.error)) {
        if (!offlineRef.current) {
          offlineRef.current = true
          setOffline(true)
          queryClient.cancelQueries()
        }
      }

      if (event.action.type === 'success' && offlineRef.current) {
        offlineRef.current = false
        setOffline(false)
      }
    })

    return unsubscribe
  }, [queryClient])

  const retry = useCallback(() => {
    offlineRef.current = false
    setOffline(false)
    void queryClient.invalidateQueries()
  }, [queryClient])

  return { offline, retry }
}
