// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ServiceBrowseRow } from './ServiceBrowseRow'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { OpsBrowsedService } from '@/types'

function service(partial: Partial<OpsBrowsedService> = {}): OpsBrowsedService {
  return {
    unit: 'app-gnome\\x2dkeyring.service',
    unitType: 'service',
    description: 'Certificate and Key Storage',
    activeState: 'inactive',
    enabledState: 'generated',
    manager: 'systemd',
    scope: 'user',
    tracked: false,
    ...partial,
  }
}

describe('ServiceBrowseRow', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows systemd escaped unit names in a readable form', () => {
    render(
      <TooltipProvider>
        <ServiceBrowseRow
          service={service()}
          pendingAction={undefined}
          onAction={vi.fn()}
          onInspect={vi.fn()}
          onLogs={vi.fn()}
          onToggleTrack={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.getByText('app-gnome-keyring.service')).toBeTruthy()
    expect(screen.queryByText('app-gnome\\x2dkeyring.service')).toBeNull()
  })

  it('keeps row actions compact and accessible', () => {
    const onAction = vi.fn()
    const onInspect = vi.fn()
    const onLogs = vi.fn()

    render(
      <TooltipProvider>
        <ServiceBrowseRow
          service={service({
            activeState: 'inactive',
            enabledState: 'disabled',
          })}
          pendingAction={undefined}
          onAction={onAction}
          onInspect={onInspect}
          onLogs={onLogs}
          onToggleTrack={vi.fn()}
        />
      </TooltipProvider>,
    )

    const startButton = screen.getByRole('button', { name: 'Start service' })

    fireEvent.click(startButton)
    fireEvent.click(screen.getByRole('button', { name: 'Inspect service status' }))
    fireEvent.click(screen.getByRole('button', { name: 'View service logs' }))

    expect(onAction).toHaveBeenCalledWith(expect.any(Object), 'start')
    expect(onInspect).toHaveBeenCalledOnce()
    expect(onLogs).toHaveBeenCalledOnce()
    expect(startButton.className).toContain('h-8')
    expect(startButton.className).toContain('sm:h-6')
    expect(screen.queryByText('Start')).toBeNull()
    expect(screen.queryByText('Restart')).toBeNull()
  })
})
