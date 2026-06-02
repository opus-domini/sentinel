// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionTabs from './SessionTabs'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

function renderTabs(overrides = {}) {
  const props = {
    openTabs: ['api', 'worker'],
    activeSession: 'api',
    activitySessions: undefined as ReadonlySet<string> | undefined,
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onReorder: vi.fn(),
    ...overrides,
  }

  render(<SessionTabs {...props} />)

  return props
}

describe('SessionTabs', () => {
  afterEach(() => {
    cleanup()
  })

  it('marks the active tab and labels close buttons per session', () => {
    renderTabs()

    expect(screen.getByRole('tab', { name: 'api' }).getAttribute('aria-selected')).toBe('true')
    expect(screen.getByRole('tab', { name: 'worker' }).getAttribute('aria-selected')).toBe('false')
    expect(screen.getByRole('button', { name: 'Close worker tab' })).toBeTruthy()
  })

  it('colors inactive tabs with activity using the primary menu tint', () => {
    renderTabs({ activitySessions: new Set(['api', 'worker']) })

    const activeTab = screen.getByRole('tab', { name: 'api' })
    const inactiveTab = screen.getByRole('tab', { name: 'worker' })
    const activeLabel = screen.getByText('api')
    const inactiveLabel = screen.getByText('worker')

    expect(activeTab.className).toContain('text-foreground')
    expect(activeLabel.className).not.toContain('text-primary/60')
    expect(inactiveTab.className).toContain('text-secondary-foreground')
    expect(inactiveLabel.className).toContain('text-primary/60')
    expect(inactiveTab.className).not.toContain('border-primary')
  })

  it('selects tabs from the keyboard without closing on delete keys', () => {
    const props = renderTabs()
    const workerTab = screen.getByRole('tab', { name: 'worker' })

    fireEvent.keyDown(workerTab, { key: 'Enter' })
    fireEvent.keyDown(workerTab, { key: ' ' })
    fireEvent.keyDown(workerTab, { key: 'Delete' })
    fireEvent.keyDown(workerTab, { key: 'Backspace' })

    expect(props.onSelect).toHaveBeenNthCalledWith(1, 'worker')
    expect(props.onSelect).toHaveBeenNthCalledWith(2, 'worker')
    expect(props.onClose).not.toHaveBeenCalled()
  })

  it('keeps the close button from selecting the tab underneath', () => {
    const props = renderTabs()

    fireEvent.click(screen.getByRole('button', { name: 'Close worker tab' }))

    expect(props.onClose).toHaveBeenCalledWith('worker')
    expect(props.onSelect).not.toHaveBeenCalled()
  })
})
