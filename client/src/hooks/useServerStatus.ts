import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'

export function useServerStatus() {
  const queryClient = useQueryClient()
  const [offline, setOffline] = useState(false)

  useEffect(() => {
    const cache = queryClient.getQueryCache()
    const unsubscribe = cache.subscribe((event) => {
      if (event.type !== 'updated') return

      if (
        event.action.type === 'error' &&
        event.action.error instanceof TypeError
      ) {
        setOffline(true)
      }

      if (event.action.type === 'success') {
        setOffline(false)
      }
    })

    return unsubscribe
  }, [queryClient])

  const retry = useCallback(() => {
    setOffline(false)
    void queryClient.invalidateQueries()
  }, [queryClient])

  return { offline, retry }
}
