type NamedWithSortOrder = {
  name: string
  sortOrder?: number
}

export function sortBySidebarOrder<T extends NamedWithSortOrder>(
  items: Array<T>,
): Array<T> {
  return [...items].sort((left, right) => {
    const leftOrder =
      typeof left.sortOrder === 'number'
        ? left.sortOrder
        : Number.MAX_SAFE_INTEGER
    const rightOrder =
      typeof right.sortOrder === 'number'
        ? right.sortOrder
        : Number.MAX_SAFE_INTEGER

    if (leftOrder !== rightOrder) {
      return leftOrder - rightOrder
    }

    return left.name.localeCompare(right.name, undefined, {
      sensitivity: 'base',
    })
  })
}

export function moveSidebarItem(
  names: Array<string>,
  activeName: string,
  overName: string,
): Array<string> {
  const from = names.indexOf(activeName)
  const to = names.indexOf(overName)
  if (from === -1 || to === -1 || from === to) {
    return names
  }

  const next = [...names]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

export function applySidebarOrder<T extends NamedWithSortOrder>(
  items: Array<T>,
  orderedNames: Array<string>,
): Array<T> {
  const orderByName = new Map(
    orderedNames.map((name, index) => [name, index + 1]),
  )
  return items.map((item) => {
    const sortOrder = orderByName.get(item.name)
    if (sortOrder === undefined || item.sortOrder === sortOrder) {
      return item
    }
    return {
      ...item,
      sortOrder,
    }
  })
}

export function nextFrontSortOrder<T extends NamedWithSortOrder>(
  items: Array<T>,
): number {
  let minOrder = Number.POSITIVE_INFINITY
  for (const item of items) {
    if (typeof item.sortOrder === 'number') {
      minOrder = Math.min(minOrder, item.sortOrder)
    }
  }
  if (!Number.isFinite(minOrder)) {
    return 1
  }
  return minOrder - 1
}
