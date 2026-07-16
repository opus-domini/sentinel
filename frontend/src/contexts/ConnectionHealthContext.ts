import { createContext, useContext } from 'react'

export type ConnectionIssue = {
  code: string
  title: string
  message: string
  configPath: string
  configuration: string
}

export type ConnectionHealth = {
  ready: boolean
  checking: boolean
  issue: ConnectionIssue | null
  retry: () => void
}

const defaultConnectionHealth: ConnectionHealth = {
  ready: true,
  checking: false,
  issue: null,
  retry: () => undefined,
}

export const ConnectionHealthContext = createContext<ConnectionHealth>(defaultConnectionHealth)

export function useConnectionHealth(): ConnectionHealth {
  return useContext(ConnectionHealthContext)
}
