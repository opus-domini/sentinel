import { describe, expect, it } from 'vitest'
import type { OpsServiceStatus } from '@/types'
import {
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
})
