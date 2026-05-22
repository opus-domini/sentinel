import { useEffect, useRef } from 'react'
import type { ConnectionState } from '@/types'
import { useOpsEventsContext } from '@/contexts/OpsEventsContext'

/**
 * Subscribe to the shared ops events WebSocket from a route component.
 * The handler is registered on mount and removed on unmount.
 * The shared connection activates on the first subscriber and
 * deactivates when all subscribers are gone.
 */
export function useOpsEvents(onMessage: (message: unknown) => void): ConnectionState {
  const { connectionState, subscribe } = useOpsEventsContext()
  const handlerRef = useRef(onMessage)
  handlerRef.current = onMessage

  useEffect(() => {
    const stableHandler = (message: unknown) => {
      handlerRef.current(message)
    }
    return subscribe(stableHandler)
  }, [subscribe])

  return connectionState
}

export function useOpsEventsReconnect(): () => void {
  const { forceReconnect } = useOpsEventsContext()
  return forceReconnect
}
