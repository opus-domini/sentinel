import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'

type MetaResponse = {
  tokenRequired?: boolean
  defaultCwd?: string
  version?: string
}

export function useSentinelMeta() {
  const metaQuery = useQuery({
    queryKey: ['meta'],
    queryFn: async ({ signal }) => {
      const response = await fetch('/api/meta', {
        signal,
        headers: { Accept: 'application/json' },
        credentials: 'same-origin',
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
        throw new Error(`meta fetch failed: HTTP ${response.status}`)
      }

      const payload = (await response.json()) as { data?: MetaResponse }
      return {
        tokenRequired: Boolean(payload.data?.tokenRequired),
        defaultCwd: (payload.data?.defaultCwd ?? '').trim(),
        version: (payload.data?.version ?? 'dev').trim() || 'dev',
        unauthorized: false,
      }
    },
    retry: 2,
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
    loaded: metaQuery.isFetched,
  }
}
