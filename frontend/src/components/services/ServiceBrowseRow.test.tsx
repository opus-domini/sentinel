// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import type { ComponentProps } from 'react'
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

function renderRow(
  props: Partial<ComponentProps<typeof ServiceBrowseRow>> & {
    service?: OpsBrowsedService
  } = {},
) {
  return render(
    <TooltipProvider>
      <ServiceBrowseRow
        service={props.service ?? service()}
        pendingAction={props.pendingAction}
        onAction={props.onAction ?? vi.fn()}
        onInspect={props.onInspect ?? vi.fn()}
        onLogs={props.onLogs ?? vi.fn()}
        onToggleTrack={props.onToggleTrack ?? vi.fn()}
      />
    </TooltipProvider>,
  )
}

describe('ServiceBrowseRow', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows systemd escaped unit names in a readable form', () => {
    renderRow()

    expect(screen.getByText('app-gnome-keyring.service')).toBeTruthy()
    expect(screen.queryByText('app-gnome\\x2dkeyring.service')).toBeNull()
  })

  it('keeps row actions compact and accessible', () => {
    const onAction = vi.fn()
    const onInspect = vi.fn()
    const onLogs = vi.fn()

    renderRow({
      service: service({
        activeState: 'inactive',
        enabledState: 'disabled',
      }),
      onAction,
      onInspect,
      onLogs,
    })

    const startButton = screen.getByRole('button', { name: 'Start service' })

    fireEvent.click(startButton)
    fireEvent.click(screen.getByRole('button', { name: 'Enable service' }))
    fireEvent.click(screen.getByRole('button', { name: 'Inspect service status' }))
    fireEvent.click(screen.getByRole('button', { name: 'View service logs' }))

    expect(onAction).toHaveBeenCalledWith(expect.any(Object), 'start')
    expect(onAction).toHaveBeenCalledWith(expect.any(Object), 'enable')
    expect(onInspect).toHaveBeenCalledOnce()
    expect(onLogs).toHaveBeenCalledOnce()
    expect(startButton.className).toContain('h-8')
    expect(startButton.className).toContain('sm:h-6')
    expect(screen.queryByText('Start')).toBeNull()
    expect(screen.queryByText('Restart')).toBeNull()
  })

  it.each([
    ['Stop service', 'stop'],
    ['Restart service', 'restart'],
    ['Disable service', 'disable'],
  ] as const)('confirms system %s action before calling onAction', (buttonName, action) => {
    const onAction = vi.fn()
    const svc = service({
      unit: 'nginx.service',
      description: 'Nginx Web Server',
      activeState: 'active',
      enabledState: 'enabled',
      manager: 'systemd',
      scope: 'system',
    })

    renderRow({ service: svc, onAction })

    fireEvent.click(screen.getByRole('button', { name: buttonName }))

    expect(onAction).not.toHaveBeenCalled()

    const dialog = screen.getByRole('alertdialog')
    expect(within(dialog).getByText(action[0].toUpperCase() + action.slice(1))).toBeTruthy()
    expect(within(dialog).getAllByText('nginx.service')).toHaveLength(2)
    expect(within(dialog).getByText('systemd')).toBeTruthy()
    expect(within(dialog).getByText('system')).toBeTruthy()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    expect(onAction).toHaveBeenCalledOnce()
    expect(onAction).toHaveBeenCalledWith(svc, action)
  })
})
