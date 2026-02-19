import { describe, expect, it } from 'vitest'
import type { OpsServiceStatus } from '@/types'
import {
  canStartOpsService,
  canStopOpsService,
  filterOpsServicesByQuery,
  isOpsServiceActive,
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'

function buildService(partial: Partial<OpsServiceStatus>): OpsServiceStatus {
  return {
    name: 'sentinel',
    displayName: 'Sentinel service',
    manager: 'systemd',
    scope: 'user',
    unit: 'sentinel',
    exists: true,
    enabledState: 'enabled',
    activeState: 'active',
    updatedAt: '2026-02-15T12:00:00Z',
    ...partial,
  }
}

describe('opsServices', () => {
  it('detects active states', () => {
    expect(isOpsServiceActive(buildService({ activeState: 'active' }))).toBe(
      true,
    )
    expect(isOpsServiceActive(buildService({ activeState: 'running' }))).toBe(
      true,
    )
    expect(isOpsServiceActive(buildService({ activeState: 'inactive' }))).toBe(
      false,
    )
  })

  it('enables start/stop actions by service state', () => {
    expect(canStartOpsService(buildService({ activeState: 'active' }))).toBe(
      false,
    )
    expect(canStartOpsService(buildService({ activeState: 'running' }))).toBe(
      false,
    )
    expect(canStartOpsService(buildService({ activeState: 'inactive' }))).toBe(
      true,
    )
    expect(canStartOpsService(buildService({ activeState: 'failed' }))).toBe(
      true,
    )

    expect(canStopOpsService(buildService({ activeState: 'active' }))).toBe(
      true,
    )
    expect(canStopOpsService(buildService({ activeState: 'running' }))).toBe(
      true,
    )
    expect(canStopOpsService(buildService({ activeState: 'inactive' }))).toBe(
      false,
    )
    expect(canStopOpsService(buildService({ activeState: 'failed' }))).toBe(
      false,
    )
  })

  it('applies optimistic action state', () => {
    expect(
      withOptimisticServiceAction(buildService({}), 'start').activeState,
    ).toBe('activating')
    expect(
      withOptimisticServiceAction(buildService({}), 'stop').activeState,
    ).toBe('stopping')
    expect(
      withOptimisticServiceAction(buildService({}), 'restart').activeState,
    ).toBe('restarting')
  })

  it('upserts a service by name', () => {
    const first = buildService({ name: 'sentinel', activeState: 'active' })
    const second = buildService({
      name: 'sentinel-updater',
      activeState: 'inactive',
    })

    const updated = upsertOpsService(
      [first, second],
      buildService({ name: 'sentinel', activeState: 'failed' }),
    )
    expect(updated).toHaveLength(2)
    expect(updated[0].activeState).toBe('failed')
  })

  it('filters services by query and keeps sorted output', () => {
    const services = [
      buildService({
        name: 'queue-worker',
        displayName: 'Queue Worker',
        unit: 'queue-worker.service',
      }),
      buildService({
        name: 'api',
        displayName: 'API',
        unit: 'api.service',
      }),
      buildService({
        name: 'sentinel-updater',
        displayName: 'Updater',
        unit: 'sentinel-updater.timer',
      }),
    ]

    const filtered = filterOpsServicesByQuery(services, 'api')
    expect(filtered).toHaveLength(1)
    expect(filtered[0].name).toBe('api')

    const sorted = filterOpsServicesByQuery(services, '')
    expect(sorted.map((service) => service.displayName)).toEqual([
      'API',
      'Queue Worker',
      'Updater',
    ])
  })
})
