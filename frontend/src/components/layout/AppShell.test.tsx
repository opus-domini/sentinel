// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import AppShell from './AppShell'
import { LayoutContext } from '@/contexts/LayoutContext'

vi.mock('../SideRail', () => ({
  default: () => <nav aria-label="side rail" />,
}))

vi.mock('@/components/settings/SettingsDialog', () => ({
  default: () => null,
}))

vi.mock('@/hooks/useEdgeSwipe', () => ({
  useEdgeSwipe: vi.fn(),
}))

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

function renderShell(overrides = {}) {
  const layoutValue = {
    sidebarOpen: false,
    setSidebarOpen: vi.fn(),
    sidebarCollapsed: false,
    setSidebarCollapsed: vi.fn(),
    sidebarDensity: 'full' as const,
    sidebarWidth: 340,
    sidebarMinWidth: 240,
    sidebarMaxWidth: 440,
    settingsOpen: false,
    setSettingsOpen: vi.fn(),
    shellStyle: {},
    layoutGridClass:
      'grid h-full grid-cols-[1fr] grid-rows-[minmax(0,1fr)] md:[grid-template-columns:48px_var(--sidebar-width)_6px_1fr]',
    startSidebarResize: vi.fn(),
    resizeSidebarBy: vi.fn(),
    resizeSidebarTo: vi.fn(),
    ...overrides,
  }

  render(
    <LayoutContext.Provider value={layoutValue}>
      <AppShell sidebar={<aside>Sessions</aside>}>
        <main>Terminal</main>
      </AppShell>
    </LayoutContext.Provider>,
  )

  return layoutValue
}

describe('AppShell', () => {
  afterEach(() => {
    cleanup()
  })

  it('exposes the sidebar resizer as a keyboard-operable separator', () => {
    const layout = renderShell()

    const separator = screen.getByRole('separator', {
      name: 'Resize sidebar',
    })

    expect(separator.getAttribute('aria-orientation')).toBe('vertical')
    expect(separator.getAttribute('aria-valuemin')).toBe('240')
    expect(separator.getAttribute('aria-valuemax')).toBe('440')
    expect(separator.getAttribute('aria-valuenow')).toBe('340')

    fireEvent.keyDown(separator, { key: 'ArrowRight' })
    fireEvent.keyDown(separator, { key: 'ArrowLeft', shiftKey: true })
    fireEvent.keyDown(separator, { key: 'Home' })
    fireEvent.keyDown(separator, { key: 'End' })

    expect(layout.resizeSidebarBy).toHaveBeenNthCalledWith(1, 16)
    expect(layout.resizeSidebarBy).toHaveBeenNthCalledWith(2, -40)
    expect(layout.resizeSidebarTo).toHaveBeenNthCalledWith(1, 240)
    expect(layout.resizeSidebarTo).toHaveBeenNthCalledWith(2, 440)
  })

  it('does not resize the sidebar for modified arrow shortcuts', () => {
    const layout = renderShell()
    const separator = screen.getByRole('separator', {
      name: 'Resize sidebar',
    })

    fireEvent.keyDown(separator, { key: 'ArrowRight', ctrlKey: true })
    fireEvent.keyDown(separator, { key: 'ArrowLeft', metaKey: true })
    fireEvent.keyDown(separator, { key: 'ArrowRight', altKey: true })

    expect(layout.resizeSidebarBy).not.toHaveBeenCalled()
    expect(layout.resizeSidebarTo).not.toHaveBeenCalled()
  })
})
