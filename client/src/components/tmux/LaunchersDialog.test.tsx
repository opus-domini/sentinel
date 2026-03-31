// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import LaunchersDialog from './LaunchersDialog'
import type { TmuxLauncher } from '@/types'

describe('LaunchersDialog', () => {
  afterEach(() => {
    cleanup()
  })

  it('applies quick start presets and saves them', async () => {
    const onSave = vi.fn().mockResolvedValue('launcher-codex')

    render(
      <LaunchersDialog
        open
        onOpenChange={vi.fn()}
        launchers={[]}
        onSave={onSave}
        onDelete={vi.fn().mockResolvedValue(true)}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.pointerDown(
      screen.getByRole('button', { name: 'Open launcher presets' }),
      { button: 0, ctrlKey: false },
    )
    fireEvent.click(await screen.findByText('Codex'))

    expect(screen.getByDisplayValue('Codex')).not.toBeNull()
    expect(screen.getAllByDisplayValue('codex')).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith({
        name: 'Codex',
        icon: 'code',
        command: 'codex',
        cwdMode: 'active-pane',
        cwdValue: '',
        windowName: 'codex',
      })
    })
  })

  it('shows a simpler empty state and keeps presets in the split button', () => {
    render(
      <LaunchersDialog
        open
        onOpenChange={vi.fn()}
        launchers={[]}
        onSave={vi.fn().mockResolvedValue('launcher-codex')}
        onDelete={vi.fn().mockResolvedValue(true)}
        onReorder={vi.fn()}
      />,
    )

    expect(screen.getByText('No launchers configured yet.')).not.toBeNull()
    expect(
      screen.getByText(
        'Start from a blank launcher or pick a preset from the split button above.',
      ),
    ).not.toBeNull()
    expect(screen.queryByRole('button', { name: 'Codex' })).toBeNull()
  })

  it('opens the icon picker inside the dialog', async () => {
    render(
      <LaunchersDialog
        open
        onOpenChange={vi.fn()}
        launchers={[]}
        onSave={vi.fn().mockResolvedValue('launcher-codex')}
        onDelete={vi.fn().mockResolvedValue(true)}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: 'Icon' }), {
      button: 0,
      ctrlKey: false,
    })

    expect(await screen.findByText('AI')).not.toBeNull()
  })

  it('deletes an existing launcher from the form', async () => {
    const onDelete = vi.fn().mockResolvedValue(true)
    const launchers: Array<TmuxLauncher> = [
      {
        id: 'launcher-claude',
        name: 'Claude Code',
        icon: 'bot',
        command: 'claude',
        cwdMode: 'active-pane',
        cwdValue: '',
        windowName: 'claude',
        sortOrder: 0,
        createdAt: '2026-03-31T12:00:00Z',
        updatedAt: '2026-03-31T12:00:00Z',
        lastUsedAt: '',
      },
    ]

    render(
      <LaunchersDialog
        open
        onOpenChange={vi.fn()}
        launchers={launchers}
        onSave={vi.fn().mockResolvedValue('launcher-claude')}
        onDelete={onDelete}
        onReorder={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('Claude Code').closest('button')!)
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith('launcher-claude')
    })
  })
})
