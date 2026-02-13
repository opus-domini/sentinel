import { useEffect, useState } from 'react'

type MetaResponse = {
  tokenRequired?: boolean
  defaultCwd?: string
}

export function useSentinelMeta(token: string) {
  const [tokenRequired, setTokenRequired] = useState(false)
  const [defaultCwd, setDefaultCwd] = useState('')
  const [unauthorized, setUnauthorized] = useState(false)

  useEffect(() => {
    const abortController = new AbortController()

    void (async () => {
      try {
        const headers: Record<string, string> = {
          Accept: 'application/json',
        }
        if (token.trim() !== '') {
          headers.Authorization = `Bearer ${token.trim()}`
        }

        const response = await fetch('/api/meta', {
          signal: abortController.signal,
          headers,
        })
        if (response.status === 401) {
          setTokenRequired(true)
          setUnauthorized(true)
          return
        }
        if (!response.ok) {
          return
        }

        const payload = (await response.json()) as { data?: MetaResponse }
        setTokenRequired(Boolean(payload.data?.tokenRequired))
        setDefaultCwd((payload.data?.defaultCwd ?? '').trim())
        setUnauthorized(false)
      } catch {
        if (abortController.signal.aborted) {
          return
        }
        // Keep default when metadata cannot be loaded.
      }
    })()

    return () => {
      abortController.abort()
    }
  }, [token])

  return {
    tokenRequired,
    defaultCwd,
    unauthorized,
  }
}
