import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'

type MetaResponse = {
  tokenRequired?: boolean
  defaultCwd?: string
  version?: string
}

export function useSentinelMeta(token: string) {
  const normalizedToken = token.trim()
  const metaQuery = useQuery({
    queryKey: ['meta', normalizedToken],
    queryFn: async ({ signal }) => {
      const headers: Record<string, string> = {
        Accept: 'application/json',
      }
      if (normalizedToken !== '') {
        headers.Authorization = `Bearer ${normalizedToken}`
      }

      const response = await fetch('/api/meta', {
        signal,
        headers,
      })
      if (response.status === 401) {
        return {
          tokenRequired: true,
          defaultCwd: '',
          version: 'dev',
          unauthorized: true,
        }
      }
      if (!response.ok) {
        return {
          tokenRequired: false,
          defaultCwd: '',
          version: 'dev',
          unauthorized: false,
        }
      }

      const payload = (await response.json()) as { data?: MetaResponse }
      return {
        tokenRequired: Boolean(payload.data?.tokenRequired),
        defaultCwd: (payload.data?.defaultCwd ?? '').trim(),
        version: (payload.data?.version ?? 'dev').trim() || 'dev',
        unauthorized: false,
      }
    },
    retry: false,
    staleTime: 60_000,
  })

  const value = useMemo(
    () =>
      metaQuery.data ?? {
        tokenRequired: false,
        defaultCwd: '',
        version: 'dev',
        unauthorized: false,
      },
    [metaQuery.data],
  )

  return {
    tokenRequired: value.tokenRequired,
    defaultCwd: value.defaultCwd,
    version: value.version,
    unauthorized: value.unauthorized,
  }
}
