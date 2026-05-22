// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionTabs from './SessionTabs'

vi.mock('@/hooks/useIsMobileLayout', () => ({
  useIsMobileLayout: () => false,
}))

function renderTabs() {
  const props = {
    openTabs: ['api', 'worker'],
    activeSession: 'api',
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onReorder: vi.fn(),
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

  it('selects and closes tabs from the keyboard', () => {
    const props = renderTabs()
    const workerTab = screen.getByRole('tab', { name: 'worker' })

    fireEvent.keyDown(workerTab, { key: 'Enter' })
    fireEvent.keyDown(workerTab, { key: ' ' })
    fireEvent.keyDown(workerTab, { key: 'Delete' })

    expect(props.onSelect).toHaveBeenNthCalledWith(1, 'worker')
    expect(props.onSelect).toHaveBeenNthCalledWith(2, 'worker')
    expect(props.onClose).toHaveBeenCalledWith('worker')
  })

  it('keeps the close button from selecting the tab underneath', () => {
    const props = renderTabs()

    fireEvent.click(screen.getByRole('button', { name: 'Close worker tab' }))

    expect(props.onClose).toHaveBeenCalledWith('worker')
    expect(props.onSelect).not.toHaveBeenCalled()
  })
})
