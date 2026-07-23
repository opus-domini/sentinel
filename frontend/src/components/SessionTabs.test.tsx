// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionTabs, { clampSessionTabTransform } from './SessionTabs'

const mobileLayout = vi.hoisted(() => ({ enabled: false }))

vi.mock('@/contexts/ViewportContext', () => ({
  useViewport: () => ({
    compactLayout: mobileLayout.enabled,
    touchCapable: mobileLayout.enabled,
    touchOptimized: mobileLayout.enabled,
  }),
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
    mobileLayout.enabled = false
  })

  it('marks the active tab and labels close buttons per session', () => {
    renderTabs()

    expect(screen.getByRole('tab', { name: 'api' }).getAttribute('aria-selected')).toBe('true')
    expect(screen.getByRole('tab', { name: 'worker' }).getAttribute('aria-selected')).toBe('false')
    expect(screen.getByRole('button', { name: 'Close worker tab' })).toBeTruthy()
  })

  it('colors the session icon for active and unread tabs without tinting the label', () => {
    renderTabs({
      activitySessions: new Set(['api', 'worker']),
      sessionIcons: new Map([
        ['api', 'terminal'],
        ['worker', 'bot'],
      ]),
    })

    const activeTab = screen.getByRole('tab', { name: 'api' })
    const inactiveTab = screen.getByRole('tab', { name: 'worker, unread activity' })
    const activeLabel = screen.getByText('api')
    const inactiveLabel = screen.getByText('worker')
    const activeIcon = activeTab.querySelector('svg')
    const inactiveIcon = inactiveTab.querySelector('svg')

    expect(activeTab.className).toContain('text-foreground')
    expect(activeIcon?.classList.contains('text-primary')).toBe(true)
    expect(activeLabel.className).not.toContain('text-activity-foreground')
    expect(inactiveTab.className).toContain('text-secondary-foreground')
    expect(inactiveIcon?.classList.contains('text-activity-foreground')).toBe(true)
    expect(inactiveLabel.className).not.toContain('text-activity-foreground')
    expect(inactiveTab.className).not.toContain('border-primary')
  })

  it('hides session icons on mobile while preserving unread activity semantics', () => {
    mobileLayout.enabled = true
    renderTabs({
      activitySessions: new Set(['worker']),
      sessionIcons: new Map([
        ['api', 'terminal'],
        ['worker', 'bot'],
      ]),
    })

    const activeTab = screen.getByRole('tab', { name: 'api' })
    const unreadTab = screen.getByRole('tab', { name: 'worker, unread activity' })

    expect(activeTab.querySelectorAll('svg')).toHaveLength(0)
    expect(unreadTab.querySelectorAll('svg')).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Close worker tab' })).toBeNull()

    fireEvent.contextMenu(unreadTab)
    expect(screen.getByText('Close tab')).toBeTruthy()
  })

  it('prevents pointer text selection without removing keyboard access', () => {
    renderTabs()

    const workerTab = screen.getByRole('tab', { name: 'worker' })

    expect(workerTab.className).toContain('select-none')
    expect(fireEvent.mouseDown(workerTab)).toBe(false)
    expect(workerTab.getAttribute('tabindex')).toBe('0')
  })

  it('uses the same optical icon and label alignment as window tabs', () => {
    renderTabs()

    const apiTab = screen.getByRole('tab', { name: 'api' })
    const apiLabel = screen.getByText('api')

    expect(apiTab.className).toContain('text-[12px]/none')
    expect(apiLabel.className).toContain('pt-[5px]')
    expect(apiLabel.className).toContain('leading-none')
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

  it('fits tabs inside the bordered row and marks only the session strip for drag scrolling', () => {
    renderTabs()

    const tabs = screen.getByRole('tablist', { name: 'Session tabs' })

    expect(tabs.className).toContain('overflow-y-hidden')
    expect(screen.getByRole('tab', { name: 'api' }).className).toContain('h-full')
    expect(screen.getByRole('tab', { name: 'api' }).className).not.toContain('h-[30px]')
    expect(tabs.getAttribute('data-sentinel-session-tabs-scroll')).toBe('true')
    expect(tabs.getAttribute('style')).toContain('overscroll-behavior-x: contain')
    expect(tabs.getAttribute('style')).toContain('overscroll-behavior-y: none')
  })

  it('keeps dragged session tabs inside the visible strip bounds', () => {
    const strip = document.createElement('div')
    strip.dataset.sentinelSessionTabsScroll = 'true'
    strip.getBoundingClientRect = () =>
      ({
        top: 0,
        left: 100,
        right: 400,
        bottom: 30,
        width: 300,
        height: 30,
        x: 100,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect

    const clamped = clampSessionTabTransform(
      {
        x: -80,
        y: 24,
        scaleX: 1,
        scaleY: 1,
      },
      {
        top: 0,
        left: 70,
        right: 180,
        bottom: 30,
        width: 110,
        height: 30,
      },
      [strip],
    )

    expect(clamped.x).toBe(30)
    expect(clamped.y).toBe(0)
  })

  it('removes vertical movement even when the session strip rect is unavailable', () => {
    const clamped = clampSessionTabTransform(
      {
        x: 32,
        y: 24,
        scaleX: 1,
        scaleY: 1,
      },
      null,
      [],
    )

    expect(clamped.x).toBe(32)
    expect(clamped.y).toBe(0)
  })
})
