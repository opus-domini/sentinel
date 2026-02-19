import { useCallback } from 'react'

export function useTmuxApi() {
  return useCallback(
    async <T>(path: string, init?: RequestInit): Promise<T> => {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      }

      if (init?.headers) {
        Object.assign(headers, init.headers as Record<string, string>)
      }

      const response = await fetch(path, {
        ...init,
        credentials: 'same-origin',
        headers,
      })

      let payload: unknown = {}
      try {
        payload = await response.json()
      } catch {
        payload = {}
      }

      if (!response.ok) {
        const message =
          typeof payload === 'object' &&
          payload !== null &&
          'error' in payload &&
          typeof (payload as { error?: { message?: string } }).error
            ?.message === 'string'
            ? (payload as { error: { message: string } }).error.message
            : `HTTP ${response.status}`
        throw new Error(message)
      }

      if (
        typeof payload === 'object' &&
        payload !== null &&
        'data' in payload
      ) {
        return (payload as { data: T }).data
      }

      return payload as T
    },
    [],
  )
}
