import { describe, expect, it } from 'vitest'

import {
  classifySessionPatches,
  inspectorRefreshModeFromSessionProjection,
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

    expect(decision).toEqual({ refresh: true, minGapMs: 20_000 })
  })

  it('refreshes when activity patches include unknown sessions', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: true,
    })

    expect(decision).toEqual({ refresh: true, minGapMs: 20_000 })
  })

  it('refreshes when activity patches are for unknown sessions only', () => {
    const decision = shouldRefreshSessionsFromEvent('activity', {
      hasInputPatches: true,
      applied: false,
      hasUnknownSession: true,
    })

    expect(decision).toEqual({ refresh: true, minGapMs: 20_000 })
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

  it('refreshes for seen action when unknown sessions are present', () => {
    const decision = shouldRefreshSessionsFromEvent('seen', {
      hasInputPatches: true,
      applied: true,
      hasUnknownSession: true,
    })

    expect(decision).toEqual({ refresh: true, minGapMs: 20_000 })
  })

  it('refreshes for non-activity actions when unknown sessions are present', () => {
    const decision = shouldRefreshSessionsFromEvent('create', {
      hasInputPatches: false,
      applied: true,
      hasUnknownSession: true,
    })

    expect(decision).toEqual({ refresh: true })
  })

  it('refreshes when action is undefined', () => {
    const decision = shouldRefreshSessionsFromEvent(undefined, {
      hasInputPatches: false,
      applied: false,
      hasUnknownSession: false,
    })

    expect(decision).toEqual({ refresh: true })
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
    expect(
      inspectorRefreshModeFromSessionProjection(
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
      ),
    ).toBe('full')
  })

  it('refreshes when unread counters change', () => {
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
    expect(
      inspectorRefreshModeFromSessionProjection(
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
      ),
    ).toBe('windows')
  })

  it('refreshes when unread counts change but structure is stable', () => {
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

    expect(decision).toBe(true)
    expect(
      inspectorRefreshModeFromSessionProjection(
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
      ),
    ).toBe('windows')
  })
})

describe('classifySessionPatches', () => {
  it('returns no-op result for undefined patches', () => {
    const result = classifySessionPatches(
      undefined,
      new Set(['dev']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: false,
      hasUnknownSession: false,
      applicableNames: [],
    })
  })

  it('returns no-op result for empty patches array', () => {
    const result = classifySessionPatches(
      [],
      new Set(['dev']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: false,
      hasUnknownSession: false,
      applicableNames: [],
    })
  })

  it('skips patches with empty or missing names', () => {
    const result = classifySessionPatches(
      [{ name: '' }, { name: undefined }, { windows: 1 }],
      new Set(['dev']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: false,
      hasUnknownSession: false,
      applicableNames: [],
    })
  })

  it('flags unknown session that is not tracked (original bug scenario)', () => {
    const result = classifySessionPatches(
      [{ name: 'new-session', windows: 1 }],
      new Set(['dev']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: true,
      applicableNames: [],
    })
  })

  it('flags unknown session that happens to be tracked', () => {
    const result = classifySessionPatches(
      [{ name: 'staging', windows: 2 }],
      new Set(['dev']),
      new Set(['dev', 'staging']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: true,
      applicableNames: [],
    })
  })

  it('applies patch for known and tracked session', () => {
    const result = classifySessionPatches(
      [{ name: 'dev', windows: 3 }],
      new Set(['dev', 'staging']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: false,
      applicableNames: ['dev'],
    })
  })

  it('skips known but untracked session (policy skip)', () => {
    const result = classifySessionPatches(
      [{ name: 'staging', windows: 2 }],
      new Set(['dev', 'staging']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: false,
      applicableNames: [],
    })
  })

  it('handles mix of known+tracked, known+untracked, and unknown', () => {
    const result = classifySessionPatches(
      [
        { name: 'dev', windows: 3 },
        { name: 'staging', windows: 2 },
        { name: 'new-session', windows: 1 },
      ],
      new Set(['dev', 'staging']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: true,
      applicableNames: ['dev'],
    })
  })

  it('flags all unknown when none are in known sessions', () => {
    const result = classifySessionPatches(
      [
        { name: 'alpha', windows: 1 },
        { name: 'beta', windows: 2 },
      ],
      new Set(['dev']),
      new Set(['dev']),
    )

    expect(result).toEqual({
      hasInputPatches: true,
      hasUnknownSession: true,
      applicableNames: [],
    })
  })
})
