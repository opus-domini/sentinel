import { QueryCache, QueryClient } from '@tanstack/react-query'

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

// --- server-status external store ----------------------------------------

let serverOffline = false
const statusListeners = new Set<() => void>()

function setServerOffline(value: boolean) {
  if (serverOffline === value) return
  serverOffline = value
  statusListeners.forEach((l) => l())
}

export function getServerOffline() {
  return serverOffline
}

export function subscribeServerStatus(listener: () => void) {
  statusListeners.add(listener)
  return () => {
    statusListeners.delete(listener)
  }
}

export function retryServer() {
  setServerOffline(false)
  void queryClient.invalidateQueries()
}

// --- query client --------------------------------------------------------

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => {
      if (isNetworkError(error)) {
        setServerOffline(true)
        void queryClient.cancelQueries()
      }
    },
    onSuccess: () => {
      if (serverOffline) {
        setServerOffline(false)
      }
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      gcTime: 30 * 60 * 1_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})
