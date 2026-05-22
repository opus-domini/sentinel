import { createContext, useContext } from 'react'
import type { ConnectionState } from '@/types'

type OpsEventsSubscriber = (message: unknown) => void

type OpsEventsContextValue = {
  connectionState: ConnectionState
  subscribe: (handler: OpsEventsSubscriber) => () => void
  forceReconnect: () => void
}

export const OpsEventsContext = createContext<OpsEventsContextValue | null>(null)

export function useOpsEventsContext(): OpsEventsContextValue {
  const value = useContext(OpsEventsContext)
  if (!value) {
    throw new Error('useOpsEventsContext must be used within an OpsEventsContext.Provider')
  }
  return value
}
