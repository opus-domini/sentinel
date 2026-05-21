import { describe, expect, it } from 'vitest'

import {
  applySidebarOrder,
  moveSidebarItem,
  nextFrontSortOrder,
  sortBySidebarOrder,
} from './sessionSidebarOrder'

describe('sortBySidebarOrder', () => {
  it('sorts by persisted sort order before name', () => {
    const ordered = sortBySidebarOrder([
      { name: 'web', sortOrder: 3 },
      { name: 'api', sortOrder: 1 },
      { name: 'docs', sortOrder: 2 },
    ])

    expect(ordered.map((item) => item.name)).toEqual(['api', 'docs', 'web'])
  })

  it('pushes unordered entries after ordered ones', () => {
    const ordered = sortBySidebarOrder([
      { name: 'web' },
      { name: 'api', sortOrder: 1 },
      { name: 'docs' },
    ])

    expect(ordered.map((item) => item.name)).toEqual(['api', 'docs', 'web'])
  })
})

describe('moveSidebarItem', () => {
  it('moves an item before the target item', () => {
    expect(moveSidebarItem(['api', 'web', 'docs'], 'docs', 'api')).toEqual([
      'docs',
      'api',
      'web',
    ])
  })

  it('returns the original order when the move is invalid', () => {
    const names = ['api', 'web']

    expect(moveSidebarItem(names, 'missing', 'api')).toBe(names)
  })
})

describe('applySidebarOrder', () => {
  it('rewrites sort orders for the provided names', () => {
    const ordered = applySidebarOrder(
      [
        { name: 'api', sortOrder: 2 },
        { name: 'web', sortOrder: 1 },
        { name: 'docs', sortOrder: 9 },
      ],
      ['docs', 'api', 'web'],
    )

    expect(ordered).toEqual([
      { name: 'api', sortOrder: 2 },
      { name: 'web', sortOrder: 3 },
      { name: 'docs', sortOrder: 1 },
    ])
  })
})

describe('nextFrontSortOrder', () => {
  it('returns a value before the current minimum', () => {
    expect(
      nextFrontSortOrder([
        { name: 'api', sortOrder: 2 },
        { name: 'web', sortOrder: 5 },
      ]),
    ).toBe(1)
  })

  it('starts at one when the list is empty', () => {
    expect(nextFrontSortOrder([])).toBe(1)
  })
})
