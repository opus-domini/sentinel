// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { OpsServiceInspect } from '@/types'
import { ServiceStatusDialog } from './ServiceStatusDialog'

afterEach(cleanup)

describe('ServiceStatusDialog', () => {
  it('hands status off to logs without adding a service action', () => {
    const onViewLogs = vi.fn()
    const data = {
      service: { unit: 'sentinel.service' },
      summary: 'active',
      checkedAt: '2026-07-27T12:00:00Z',
      properties: {},
      output: '',
    } as OpsServiceInspect

    render(
      <ServiceStatusDialog
        open
        onOpenChange={vi.fn()}
        loading={false}
        error=""
        data={data}
        onViewLogs={onViewLogs}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'View logs' }))

    expect(onViewLogs).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: /restart|start|stop/i })).toBeNull()
  })
})
