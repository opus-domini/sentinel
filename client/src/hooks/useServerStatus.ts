import { useSyncExternalStore } from 'react'
import {
  getServerOffline,
  retryServer,
  subscribeServerStatus,
} from '@/lib/queryClient'

export function useServerStatus() {
  const offline = useSyncExternalStore(subscribeServerStatus, getServerOffline)
  return { offline, retry: retryServer }
}
