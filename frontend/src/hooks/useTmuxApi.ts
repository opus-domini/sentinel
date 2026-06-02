import { useCallback } from 'react'

export function useTmuxApi() {
  return useCallback(async <T>(path: string, init?: RequestInit): Promise<T> => {
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
      const errorObj =
        typeof payload === 'object' && payload !== null && 'error' in payload
          ? (payload as { error: Record<string, unknown> }).error
          : null

      const message =
        errorObj?.message != null && typeof errorObj.message === 'string'
          ? errorObj.message
          : `HTTP ${response.status}`
      throw new Error(message)
    }

    if (typeof payload === 'object' && payload !== null && 'data' in payload) {
      return (payload as { data: T }).data
    }

    return payload as T
  }, [])
}
