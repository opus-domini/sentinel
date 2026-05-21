import type {
  OpsBrowsedService,
  OpsServiceAction,
  OpsServiceStatus,
} from '@/types'

const opsBrowseUnitTypeOrder = [
  'service',
  'timer',
  'socket',
  'target',
  'path',
  'mount',
  'automount',
  'swap',
  'slice',
  'scope',
  'job',
  'unit',
]

type HasActiveState = { activeState: string }
type HasTrackingState = { tracked: boolean }

export type OpsServiceStateFilter =
  | 'all'
  | 'active'
  | 'inactive'
  | 'failed'
  | 'changing'

export type OpsServiceTrackFilter = 'all' | 'tracked' | 'untracked'

const systemdHexEscapePattern = /(?:\\x[0-9a-fA-F]{2})+/g
const systemdHexBytePattern = /\\x([0-9a-fA-F]{2})/g

function normalizedBrowseUnitType(raw: string): string {
  const type = raw.trim().toLowerCase()
  return type === '' ? 'unit' : type
}

function compareBrowseUnitTypes(left: string, right: string): number {
  const leftIndex = opsBrowseUnitTypeOrder.indexOf(left)
  const rightIndex = opsBrowseUnitTypeOrder.indexOf(right)
  if (leftIndex !== -1 || rightIndex !== -1) {
    if (leftIndex === -1) return 1
    if (rightIndex === -1) return -1
    return leftIndex - rightIndex
  }
  return left.localeCompare(right, undefined, { sensitivity: 'base' })
}

function normalizedActiveState(service: HasActiveState): string {
  return service.activeState.trim().toLowerCase()
}

export function isOpsServiceChanging(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return (
    state === 'activating' ||
    state === 'deactivating' ||
    state === 'reloading' ||
    state === 'restarting' ||
    state === 'stopping'
  )
}

export function isOpsServiceActive(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return state === 'active' || state === 'running'
}

export function isOpsServiceFailed(service: HasActiveState): boolean {
  return normalizedActiveState(service) === 'failed'
}

export function isOpsServiceInactive(service: HasActiveState): boolean {
  const state = normalizedActiveState(service)
  return state === 'inactive' || state === 'dead'
}

export function matchesOpsServiceStateFilter(
  service: HasActiveState,
  filter: OpsServiceStateFilter,
): boolean {
  if (filter === 'all') return true
  if (filter === 'active') return isOpsServiceActive(service)
  if (filter === 'inactive') return isOpsServiceInactive(service)
  if (filter === 'failed') return isOpsServiceFailed(service)
  if (filter === 'changing') return isOpsServiceChanging(service)
  return true
}

export function matchesOpsServiceTrackFilter(
  service: HasTrackingState,
  filter: OpsServiceTrackFilter,
): boolean {
  if (filter === 'all') return true
  return filter === 'tracked' ? service.tracked : !service.tracked
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
    case 'enable':
      return {
        ...service,
        enabledState: 'enabled',
      }
    case 'disable':
      return {
        ...service,
        enabledState: 'disabled',
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

export function sortOpsBrowseUnitTypes(types: Array<string>): Array<string> {
  return [...new Set(types.map(normalizedBrowseUnitType))].sort(
    compareBrowseUnitTypes,
  )
}

export function listOpsBrowseUnitTypes(
  services: Array<Pick<OpsBrowsedService, 'unitType'>>,
): Array<string> {
  const types = services.map((service) => service.unitType)
  return sortOpsBrowseUnitTypes(types)
}

export function defaultOpsBrowseUnitTypes(types: Array<string>): Array<string> {
  const normalized = sortOpsBrowseUnitTypes(types)
  if (normalized.length === 0) return []
  if (normalized.includes('service')) return ['service']
  return normalized
}

export function formatOpsUnitName(unit: string): string {
  return unit.replace(systemdHexEscapePattern, (sequence) => {
    const bytes = Array.from(
      sequence.matchAll(systemdHexBytePattern),
      (match) => Number.parseInt(match[1], 16),
    )
    if (bytes.length === 0) return sequence
    return new TextDecoder().decode(new Uint8Array(bytes))
  })
}

export function deriveOpsTrackedServiceName(unit: string): string {
  return unit
    .trim()
    .replace(
      /\.(service|timer|socket|target|path|mount|automount|swap|slice|scope)$/,
      '',
    )
    .replace(/\./g, '-')
}
