// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RunbookRunDialog } from './RunbookRunDialog'
import type { OpsRunbook } from '@/types'

function runbook(overrides: Partial<OpsRunbook> = {}): OpsRunbook {
  return {
    id: 'runbook-1',
    name: 'Deploy',
    description: '',
    enabled: true,
    parameters: [],
    steps: [],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('RunbookRunDialog', () => {
  afterEach(() => {
    cleanup()
  })

  it('clears previous parameter values when the next runbook has no parameters', () => {
    const onConfirm = vi.fn()
    const { rerender } = render(
      <RunbookRunDialog
        open
        runbook={runbook({
          parameters: [
            {
              name: 'branch',
              label: 'Branch',
              type: 'string',
              default: 'main',
              required: false,
            },
          ],
        })}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />,
    )

    fireEvent.change(screen.getByDisplayValue('main'), {
      target: { value: 'feature' },
    })

    rerender(
      <RunbookRunDialog
        open
        runbook={runbook({ id: 'runbook-2', name: 'Ping' })}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Run' }))

    expect(onConfirm).toHaveBeenCalledWith({})
  })
})
