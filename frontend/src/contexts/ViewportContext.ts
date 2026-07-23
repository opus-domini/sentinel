import { createContext, useContext } from 'react'
import type { ViewportCapabilities } from '@/hooks/useViewportCapabilities'

export const ViewportContext = createContext<ViewportCapabilities | null>(null)

export function useViewport(): ViewportCapabilities {
  const context = useContext(ViewportContext)
  if (context === null) {
    throw new Error('useViewport must be used within ViewportContext.Provider')
  }
  return context
}
