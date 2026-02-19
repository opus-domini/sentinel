import { describe, expect, it } from 'vitest'

import { initialTabsState, tabsReducer } from './tabsReducer'
import type { TabsState } from './tabsReducer'

function state(partial: Partial<TabsState> = {}): TabsState {
  return { ...initialTabsState, ...partial }
}

describe('tabsReducer', () => {
  describe('activate', () => {
    it('adds tab and sets active session', () => {
      const result = tabsReducer(initialTabsState, {
        type: 'activate',
        session: 'dev',
      })
      expect(result.openTabs).toEqual(['dev'])
      expect(result.activeSession).toBe('dev')
      expect(result.activeEpoch).toBe(1)
    })

    it('does not duplicate existing tab', () => {
      const s = state({ openTabs: ['dev', 'prod'], activeSession: 'dev' })
      const result = tabsReducer(s, { type: 'activate', session: 'prod' })
      expect(result.openTabs).toEqual(['dev', 'prod'])
      expect(result.activeSession).toBe('prod')
    })

    it('appends new tab at the end', () => {
      const s = state({ openTabs: ['dev'], activeSession: 'dev' })
      const result = tabsReducer(s, { type: 'activate', session: 'prod' })
      expect(result.openTabs).toEqual(['dev', 'prod'])
      expect(result.activeSession).toBe('prod')
    })

    it('ignores empty session name', () => {
      const result = tabsReducer(initialTabsState, {
        type: 'activate',
        session: '',
      })
      expect(result).toBe(initialTabsState)
    })

    it('increments epoch on activate', () => {
      const s = state({ activeEpoch: 5 })
      const result = tabsReducer(s, { type: 'activate', session: 'dev' })
      expect(result.activeEpoch).toBe(6)
    })
  })

  describe('close', () => {
    it('removes tab and falls back to last remaining', () => {
      const s = state({
        openTabs: ['a', 'b', 'c'],
        activeSession: 'b',
      })
      const result = tabsReducer(s, { type: 'close', session: 'b' })
      expect(result.openTabs).toEqual(['a', 'c'])
      expect(result.activeSession).toBe('c')
    })

    it('keeps active session if closing a non-active tab', () => {
      const s = state({
        openTabs: ['a', 'b', 'c'],
        activeSession: 'a',
      })
      const result = tabsReducer(s, { type: 'close', session: 'c' })
      expect(result.openTabs).toEqual(['a', 'b'])
      expect(result.activeSession).toBe('a')
    })

    it('clears active when closing the only tab', () => {
      const s = state({ openTabs: ['solo'], activeSession: 'solo' })
      const result = tabsReducer(s, { type: 'close', session: 'solo' })
      expect(result.openTabs).toEqual([])
      expect(result.activeSession).toBe('')
    })

    it('ignores empty session name', () => {
      const s = state({ openTabs: ['a'], activeSession: 'a' })
      const result = tabsReducer(s, { type: 'close', session: '' })
      expect(result).toBe(s)
    })
  })

  describe('rename', () => {
    it('renames tab and updates active session', () => {
      const s = state({
        openTabs: ['old', 'other'],
        activeSession: 'old',
      })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: 'old',
        newName: 'new',
      })
      expect(result.openTabs).toEqual(['new', 'other'])
      expect(result.activeSession).toBe('new')
    })

    it('renames tab without changing active if not the active one', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'a',
      })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: 'b',
        newName: 'c',
      })
      expect(result.openTabs).toEqual(['a', 'c'])
      expect(result.activeSession).toBe('a')
    })

    it('deduplicates if rename creates a duplicate', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'a',
      })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: 'b',
        newName: 'a',
      })
      expect(result.openTabs).toEqual(['a'])
    })

    it('ignores empty oldName', () => {
      const s = state({ openTabs: ['a'], activeSession: 'a' })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: '',
        newName: 'b',
      })
      expect(result).toBe(s)
    })

    it('ignores empty newName', () => {
      const s = state({ openTabs: ['a'], activeSession: 'a' })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: 'a',
        newName: '',
      })
      expect(result).toBe(s)
    })

    it('ignores same name rename', () => {
      const s = state({ openTabs: ['a'], activeSession: 'a' })
      const result = tabsReducer(s, {
        type: 'rename',
        oldName: 'a',
        newName: 'a',
      })
      expect(result).toBe(s)
    })
  })

  describe('reorder', () => {
    it('swaps two tabs', () => {
      const s = state({ openTabs: ['a', 'b', 'c'], activeSession: 'a' })
      const result = tabsReducer(s, { type: 'reorder', from: 0, to: 2 })
      expect(result.openTabs).toEqual(['b', 'c', 'a'])
      expect(result.activeSession).toBe('a')
    })

    it('ignores same index', () => {
      const s = state({ openTabs: ['a', 'b'], activeSession: 'a' })
      const result = tabsReducer(s, { type: 'reorder', from: 1, to: 1 })
      expect(result).toBe(s)
    })

    it('ignores negative from', () => {
      const s = state({ openTabs: ['a', 'b'], activeSession: 'a' })
      const result = tabsReducer(s, { type: 'reorder', from: -1, to: 0 })
      expect(result).toBe(s)
    })

    it('ignores out-of-bounds to', () => {
      const s = state({ openTabs: ['a', 'b'], activeSession: 'a' })
      const result = tabsReducer(s, { type: 'reorder', from: 0, to: 5 })
      expect(result).toBe(s)
    })
  })

  describe('sync', () => {
    it('removes tabs not in session list', () => {
      const s = state({
        openTabs: ['a', 'b', 'c'],
        activeSession: 'a',
      })
      const result = tabsReducer(s, {
        type: 'sync',
        sessions: ['a', 'c'],
      })
      expect(result.openTabs).toEqual(['a', 'c'])
      expect(result.activeSession).toBe('a')
    })

    it('falls back to last tab when active is removed', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'b',
      })
      const result = tabsReducer(s, {
        type: 'sync',
        sessions: ['a'],
      })
      expect(result.openTabs).toEqual(['a'])
      expect(result.activeSession).toBe('a')
    })

    it('clears active when all tabs removed', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'a',
      })
      const result = tabsReducer(s, { type: 'sync', sessions: [] })
      expect(result.openTabs).toEqual([])
      expect(result.activeSession).toBe('')
    })

    it('does not increment epoch when active unchanged', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'a',
        activeEpoch: 3,
      })
      const result = tabsReducer(s, {
        type: 'sync',
        sessions: ['a', 'b'],
      })
      expect(result.activeEpoch).toBe(3)
    })

    it('increments epoch when active changes', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'b',
        activeEpoch: 3,
      })
      const result = tabsReducer(s, {
        type: 'sync',
        sessions: ['a'],
      })
      expect(result.activeEpoch).toBe(4)
    })
  })

  describe('clear', () => {
    it('empties all tabs and active session', () => {
      const s = state({
        openTabs: ['a', 'b'],
        activeSession: 'a',
        activeEpoch: 5,
      })
      const result = tabsReducer(s, { type: 'clear' })
      expect(result.openTabs).toEqual([])
      expect(result.activeSession).toBe('')
      expect(result.activeEpoch).toBe(6)
    })
  })

  describe('unknown action', () => {
    it('returns state unchanged', () => {
      // @ts-expect-error testing unknown action type
      const result = tabsReducer(initialTabsState, { type: 'bogus' })
      expect(result).toBe(initialTabsState)
    })
  })
})
