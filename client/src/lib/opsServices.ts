import type { OpsServiceAction, OpsServiceStatus } from '@/types'

export function isOpsServiceActive(service: OpsServiceStatus): boolean {
  const state = service.activeState.trim().toLowerCase()
  return state === 'active' || state === 'running'
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
