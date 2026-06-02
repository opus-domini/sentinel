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

  it('keeps the desktop action row accessible by label', () => {
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

    const desktop = within(screen.getByTestId('service-actions-desktop'))
    fireEvent.click(desktop.getByRole('button', { name: 'Start' }))
    fireEvent.click(desktop.getByRole('button', { name: 'Enable' }))
    fireEvent.click(desktop.getByRole('button', { name: 'Inspect status' }))
    fireEvent.click(desktop.getByRole('button', { name: 'View logs' }))

    expect(onAction).toHaveBeenCalledWith(expect.any(Object), 'start')
    expect(onAction).toHaveBeenCalledWith(expect.any(Object), 'enable')
    expect(onInspect).toHaveBeenCalledOnce()
    expect(onLogs).toHaveBeenCalledOnce()
  })

  it('collapses actions into a primary action plus overflow on mobile', () => {
    renderRow({
      service: service({ activeState: 'inactive', enabledState: 'disabled' }),
    })

    // The eight desktop icons collapse to just the inline primary action and a
    // single "More actions" overflow trigger, keeping each card short.
    const mobile = within(screen.getByTestId('service-actions-mobile'))
    expect(mobile.getByRole('button', { name: 'Start' })).toBeTruthy()
    expect(mobile.getByRole('button', { name: 'More actions' })).toBeTruthy()
    expect(mobile.getAllByRole('button')).toHaveLength(2)
  })

  it.each([
    ['Stop', 'stop'],
    ['Restart', 'restart'],
    ['Disable', 'disable'],
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

    const desktop = within(screen.getByTestId('service-actions-desktop'))
    fireEvent.click(desktop.getByRole('button', { name: buttonName }))

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
