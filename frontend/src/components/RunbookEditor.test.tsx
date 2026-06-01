// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { RunbookEditor } from './RunbookEditor'
import type { RunbookDraft } from './RunbookEditor'

const draft: RunbookDraft = {
  id: null,
  name: '',
  description: '',
  enabled: true,
  webhookURL: 'bad-url',
  parameters: [],
  steps: [],
}

describe('RunbookEditor', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
  })

  afterEach(() => cleanup())

  it('renders field errors as alerts with invalid ARIA and focuses the first invalid field', async () => {
    render(
      <RunbookEditor
        draft={draft}
        saving={false}
        errors={{ name: 'Name is required', webhookURL: 'Webhook URL is invalid' }}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )

    const name = screen.getByLabelText('Name')
    expect(screen.getByText('Name is required').getAttribute('role')).toBe('alert')
    expect(name.getAttribute('aria-invalid')).toBe('true')
    expect(name.getAttribute('aria-describedby')).toBe(screen.getByText('Name is required').id)
    expect(screen.getByLabelText('Webhook URL').getAttribute('aria-invalid')).toBe('true')
    await waitFor(() => expect(document.activeElement).toBe(name))
  })

  it('marks invalid sections and focuses the first invalid section', async () => {
    render(
      <RunbookEditor
        draft={draft}
        saving={false}
        errors={{ parameters: 'Add at least one parameter', steps: 'Add at least one step' }}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )

    const parameters = screen.getByText('Parameters (0)').closest('[aria-invalid="true"]')
    expect(parameters).toBeTruthy()
    expect(parameters?.getAttribute('aria-describedby')).toBe(
      screen.getByText('Add at least one parameter').id,
    )
    expect(screen.getByText('Add at least one parameter').getAttribute('role')).toBe('alert')
    expect(screen.getByText('Add at least one step').getAttribute('role')).toBe('alert')
    await waitFor(() => expect(document.activeElement).toBe(parameters))
  })
})
