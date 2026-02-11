import { createContext, useContext } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react'

type LayoutContextValue = {
  sidebarOpen: boolean
  setSidebarOpen: (open: boolean | ((prev: boolean) => boolean)) => void
  sidebarCollapsed: boolean
  setSidebarCollapsed: (
    collapsed: boolean | ((prev: boolean) => boolean),
  ) => void
  shellStyle: CSSProperties
  layoutGridClass: string
  startSidebarResize: (event: ReactMouseEvent<HTMLDivElement>) => void
}

export const LayoutContext = createContext<LayoutContextValue | null>(null)

export function useLayoutContext(): LayoutContextValue {
  const value = useContext(LayoutContext)
  if (!value) {
    throw new Error(
      'useLayoutContext must be used within a LayoutContext.Provider',
    )
  }
  return value
}
