import type { OpsServiceAction, OpsServiceStatus } from '@/types'

function normalizedServiceState(service: OpsServiceStatus): string {
  return service.activeState.trim().toLowerCase()
}

export function isOpsServiceActive(service: OpsServiceStatus): boolean {
  const state = normalizedServiceState(service)
  return state === 'active' || state === 'running'
}

export function canStartOpsService(service: OpsServiceStatus): boolean {
  const state = normalizedServiceState(service)
  return !(
    state === 'active' ||
    state === 'running' ||
    state === 'activating' ||
    state === 'reloading' ||
    state === 'restarting'
  )
}

export function canStopOpsService(service: OpsServiceStatus): boolean {
  const state = normalizedServiceState(service)
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
