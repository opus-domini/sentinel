// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
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
})
