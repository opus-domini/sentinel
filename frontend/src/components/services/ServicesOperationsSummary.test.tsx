// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ServicesOperationsSummary } from './ServicesOperationsSummary'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { OpsBrowsedService } from '@/types'

function service(partial: Partial<OpsBrowsedService> = {}): OpsBrowsedService {
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

function renderSummary(props: Partial<Parameters<typeof ServicesOperationsSummary>[0]> = {}) {
  const onStateFilterChange = vi.fn()
  const onTrackFilterChange = vi.fn()
  render(
    <TooltipProvider>
      <ServicesOperationsSummary
        services={[
          service({
            unit: 'nginx.service',
            description: 'Nginx',
            activeState: 'active',
          }),
          service({
            unit: 'postgresql.service',
            description: 'PostgreSQL',
            activeState: 'failed',
          }),
          service({
            unit: 'backup.service',
            description: 'Nightly backup',
            activeState: 'restarting',
          }),
          service({
            unit: 'cups.service',
            description: 'Printer',
            activeState: 'inactive',
          }),
        ]}
        trackingServices={[
          service({
            unit: 'sentinel.service',
            description: 'Sentinel',
            tracked: true,
          }),
        ]}
        stateFilter="all"
        trackFilter="all"
        onStateFilterChange={onStateFilterChange}
        onTrackFilterChange={onTrackFilterChange}
        {...props}
      />
    </TooltipProvider>,
  )
  return { onStateFilterChange, onTrackFilterChange }
}

describe('ServicesOperationsSummary', () => {
  afterEach(() => {
    cleanup()
  })

  it('summarizes operational service states and applies filters', () => {
    const { onStateFilterChange, onTrackFilterChange } = renderSummary()

    fireEvent.click(screen.getByRole('button', { name: /Failed units: 1/i }))
    fireEvent.click(screen.getByRole('button', { name: /Changing units: 1/i }))
    fireEvent.click(screen.getByRole('button', { name: /Active units: 1/i }))
    fireEvent.click(screen.getByRole('button', { name: /Stopped units: 1/i }))
    fireEvent.click(screen.getByRole('button', { name: /Pinned units: 1/i }))

    expect(onStateFilterChange).toHaveBeenNthCalledWith(1, 'failed')
    expect(onStateFilterChange).toHaveBeenNthCalledWith(2, 'changing')
    expect(onStateFilterChange).toHaveBeenNthCalledWith(3, 'active')
    expect(onStateFilterChange).toHaveBeenNthCalledWith(4, 'inactive')
    expect(onTrackFilterChange).toHaveBeenCalledWith('tracked')
  })

  it('keeps empty cards disabled unless they are selected', () => {
    const { onStateFilterChange } = renderSummary({
      services: [service({ activeState: 'active' })],
      trackingServices: [],
    })

    const failedCard = screen.getByRole('button', {
      name: /Failed units: 0/i,
    })

    expect(failedCard.getAttribute('aria-disabled')).toBe('true')
    fireEvent.click(failedCard)
    expect(onStateFilterChange).not.toHaveBeenCalled()
  })
})
