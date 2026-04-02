import { createContext, useContext } from 'react'

type MetaContextValue = {
  tokenRequired: boolean
  defaultCwd: string
  version: string
  timezone: string
  locale: string
  hostname: string
  processUser: string
  isRoot: boolean
  canSwitchUser: boolean
  allowedUsers: Array<string>
  unauthorized: boolean
  loaded: boolean
}

export const MetaContext = createContext<MetaContextValue | null>(null)

export function useMetaContext(): MetaContextValue {
  const value = useContext(MetaContext)
  if (!value) {
    throw new Error('useMetaContext must be used within a MetaContext.Provider')
  }
  return value
}
