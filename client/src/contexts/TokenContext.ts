import { createContext, useContext } from 'react'

type TokenContextValue = {
  authenticated: boolean
  setToken: (token: string) => void
}

export const TokenContext = createContext<TokenContextValue | null>(null)

export function useTokenContext(): TokenContextValue {
  const value = useContext(TokenContext)
  if (!value) {
    throw new Error(
      'useTokenContext must be used within a TokenContext.Provider',
    )
  }
  return value
}
