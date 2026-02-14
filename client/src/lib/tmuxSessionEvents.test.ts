import { describe, expect, it } from 'vitest'

import {
  shouldRefreshInspectorFromSessionProjection,
  shouldRefreshSessionsFromEvent,
} from './tmuxSessionEvents'

describe('shouldRefreshSessionsFromEvent', () => {
  it('skips refresh for activity when patches are applied for known sessions', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: false })
  })

  it('uses slow fallback refresh when activity payload arrives without patches', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: false,
      applied: false,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: true, minGapMs: 12_000 })
  })

  it('uses short-gap refresh when activity patches include unknown sessions', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: true,
    })

    expect(decision).toEqual({ refresh: true, minGapMs: 2_500 })
  })

  it('treats seen like activity for refresh decisions', () => {
    const decision = shouldRefreshSessionsFromEvent('seen', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: false })
  })

  it('skips refresh for non-activity actions when patches are sufficient', () => {
    const decision = shouldRefreshSessionsFromEvent('rename', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: false })
  })

  it('refreshes for non-activity actions when no patches are available', () => {
    const decision = shouldRefreshSessionsFromEvent('rename', {
      hasInputPatches: false,
      applied: false,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: true })
  })

  it('skips refresh for activity when patches exist but were ignored by policy', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: true,
      applied: false,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: false })
  })
})

describe('shouldRefreshInspectorFromSessionProjection', () => {
  it('does not refresh when there is no previous projection', () => {
    const decision = shouldRefreshInspectorFromSessionProjection(null, {
      name: 'dev',
      windows: 2,
      panes: 3,
      unreadWindows: 0,
      unreadPanes: 0,
    })

    expect(decision).toBe(false)
  })

  it('refreshes when window or pane structure changes', () => {
    const decision = shouldRefreshInspectorFromSessionProjection(
      {
        name: 'dev',
        windows: 2,
        panes: 3,
        unreadWindows: 0,
        unreadPanes: 0,
      },
      {
        name: 'dev',
        windows: 3,
        panes: 3,
        unreadWindows: 0,
        unreadPanes: 0,
      },
    )

    expect(decision).toBe(true)
  })

  it('refreshes when unread edge changes', () => {
    const decision = shouldRefreshInspectorFromSessionProjection(
      {
        name: 'dev',
        windows: 3,
        panes: 6,
        unreadWindows: 0,
        unreadPanes: 0,
      },
      {
        name: 'dev',
        windows: 3,
        panes: 6,
        unreadWindows: 1,
        unreadPanes: 1,
      },
    )

    expect(decision).toBe(true)
  })

  it('does not refresh when unread counts change but edge is stable', () => {
    const decision = shouldRefreshInspectorFromSessionProjection(
      {
        name: 'dev',
        windows: 3,
        panes: 6,
        unreadWindows: 1,
        unreadPanes: 1,
      },
      {
        name: 'dev',
        windows: 3,
        panes: 6,
        unreadWindows: 2,
        unreadPanes: 3,
      },
    )

    expect(decision).toBe(false)
  })
})
