import { createContext, useContext } from 'react'
import type { ToastMessage } from '../hooks/useToasts'

type ToastContextValue = {
  toasts: Array<ToastMessage>
  pushToast: (toast: {
    level: 'success' | 'error' | 'info'
    title: string
    message: string
    ttlMs?: number
  }) => void
  dismissToast: (id: number) => void
}

export const ToastContext = createContext<ToastContextValue | null>(null)

export function useToastContext(): ToastContextValue {
  const value = useContext(ToastContext)
  if (!value) {
    throw new Error(
      'useToastContext must be used within a ToastContext.Provider',
    )
  }
  return value
}
