import type { OpsServiceAction, OpsServiceStatus } from '@/types'

type HasActiveState = { activeState: string }

function normalizedActiveState(service: HasActiveState): string {
  return service.activeState.trim().toLowerCase()
}

export function isOpsServiceActive(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return state === 'active' || state === 'running'
}

export function canStartOpsService(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return !(
    state === 'active' ||
    state === 'running' ||
    state === 'activating' ||
    state === 'reloading' ||
    state === 'restarting'
  )
}

export function canStopOpsService(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return (
    state === 'active' ||
    state === 'running' ||
    state === 'activating' ||
    state === 'reloading'
  )
}

export function withOptimisticServiceAction(
  service: OpsServiceStatus,
  action: OpsServiceAction,
): OpsServiceStatus {
  switch (action) {
    case 'start':
      return {
        ...service,
        activeState: 'activating',
      }
    case 'stop':
      return {
        ...service,
        activeState: 'stopping',
      }
    case 'restart':
      return {
        ...service,
        activeState: 'restarting',
      }
  }
}

export function upsertOpsService(
  services: Array<OpsServiceStatus>,
  service: OpsServiceStatus,
): Array<OpsServiceStatus> {
  const index = services.findIndex((item) => item.name === service.name)
  if (index === -1) return [...services, service]
  return services.map((item, i) => (i === index ? service : item))
}

export function filterOpsServicesByQuery(
  services: Array<OpsServiceStatus>,
  query: string,
): Array<OpsServiceStatus> {
  const sorted = [...services].sort((left, right) => {
    const displayNameCompare = left.displayName.localeCompare(
      right.displayName,
      undefined,
      { sensitivity: 'base' },
    )
    if (displayNameCompare !== 0) return displayNameCompare
    return left.unit.localeCompare(right.unit, undefined, {
      sensitivity: 'base',
    })
  })
  const normalizedQuery = query.trim().toLowerCase()
  if (normalizedQuery === '') return sorted
  return sorted.filter((service) => {
    return (
      service.displayName.toLowerCase().includes(normalizedQuery) ||
      service.unit.toLowerCase().includes(normalizedQuery) ||
      service.name.toLowerCase().includes(normalizedQuery)
    )
  })
}
