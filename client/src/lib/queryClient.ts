import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      gcTime: 30 * 60 * 1_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})
