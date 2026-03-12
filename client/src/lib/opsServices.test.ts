import { describe, expect, it } from 'vitest'
import type { OpsBrowsedService, OpsServiceStatus } from '@/types'
import {
  canStartOpsService,
  canStopOpsService,
  defaultOpsBrowseUnitTypes,
  deriveOpsTrackedServiceName,
  filterOpsServicesByQuery,
  isOpsServiceActive,
  listOpsBrowseUnitTypes,
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

function buildBrowsedService(
  partial: Partial<OpsBrowsedService>,
): OpsBrowsedService {
  return {
    unit: 'nginx.service',
    unitType: 'service',
    description: 'Nginx',
    activeState: 'active',
    enabledState: 'enabled',
    manager: 'systemd',
    scope: 'system',
    tracked: false,
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

  it('lists browse unit types in stable order', () => {
    const types = listOpsBrowseUnitTypes([
      buildBrowsedService({ unitType: 'target' }),
      buildBrowsedService({ unitType: 'service' }),
      buildBrowsedService({ unitType: 'job', manager: 'launchd' }),
      buildBrowsedService({ unitType: 'timer' }),
      buildBrowsedService({ unitType: 'service' }),
    ])

    expect(types).toEqual(['service', 'timer', 'target', 'job'])
  })

  it('defaults browse unit type selection to service when available', () => {
    expect(defaultOpsBrowseUnitTypes(['timer', 'service', 'target'])).toEqual([
      'service',
    ])
    expect(defaultOpsBrowseUnitTypes(['job'])).toEqual(['job'])
    expect(defaultOpsBrowseUnitTypes([])).toEqual([])
  })

  it('derives tracked service names from unit names across unit types', () => {
    expect(deriveOpsTrackedServiceName('nginx.service')).toBe('nginx')
    expect(deriveOpsTrackedServiceName('backup.timer')).toBe('backup')
    expect(deriveOpsTrackedServiceName('multi-user.target')).toBe('multi-user')
    expect(deriveOpsTrackedServiceName('io.opusdomini.sentinel')).toBe(
      'io-opusdomini-sentinel',
    )
  })
})
