// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RunbookDetailPanel } from './RunbookDetailPanel'
import type { OpsRunbook } from '@/types'
import { MetaContext } from '@/contexts/MetaContext'
import { TooltipProvider } from '@/components/ui/tooltip'

afterEach(() => {
  cleanup()
})

const meta = {
  tokenRequired: false,
  defaultCwd: '',
  version: 'test',
  timezone: 'UTC',
  locale: 'en-US',
  hostname: 'sentinel.test',
  processUser: 'sentinel',
  userSwitchMethod: '',
  isRoot: false,
  canSwitchUser: false,
  allowedUsers: [],
  unauthorized: false,
  loaded: true,
}

function renderRunbook(runbook: OpsRunbook, onDelete = vi.fn()) {
  render(
    <MetaContext.Provider value={meta}>
      <TooltipProvider>
        <RunbookDetailPanel
          runbook={runbook}
          lastJob={null}
          schedule={null}
          editingSchedule={null}
          scheduleSaving={false}
          onEdit={vi.fn()}
          onDelete={onDelete}
          onRun={vi.fn()}
          onEditSchedule={vi.fn()}
          onCancelScheduleEdit={vi.fn()}
          onSaveSchedule={vi.fn()}
          onDeleteSchedule={vi.fn()}
          onToggleScheduleEnabled={vi.fn()}
          onTriggerSchedule={vi.fn()}
        />
      </TooltipProvider>
    </MetaContext.Provider>,
  )

  return onDelete
}

describe('RunbookDetailPanel', () => {
  it('allows deleting a runbook installed by default', () => {
    const runbook: OpsRunbook = {
      id: 'ops.service.recover',
      name: 'Service Recovery',
      description: 'Recover the Sentinel service',
      enabled: true,
      parameters: [],
      steps: [],
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    const onDelete = renderRunbook(runbook)
    const deleteButton = screen.getByRole('button', { name: 'Delete' })

    expect(deleteButton.hasAttribute('disabled')).toBe(false)
    fireEvent.click(deleteButton)
    expect(onDelete).toHaveBeenCalledWith(runbook)
  })
})
