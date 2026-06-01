// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ToastViewport from './ToastViewport'

describe('ToastViewport', () => {
  afterEach(() => cleanup())

  it('uses assertive alert semantics for error toasts', () => {
    render(
      <ToastViewport
        toasts={[{ id: 1, level: 'error', title: 'Failed', message: 'Could not save' }]}
        onDismiss={vi.fn()}
      />,
    )

    const toast = screen.getByRole('alert')
    expect(toast.getAttribute('aria-live')).toBe('assertive')
    expect(toast.textContent).toContain('Could not save')
  })

  it('uses polite status semantics for non-error toasts', () => {
    render(
      <ToastViewport
        toasts={[{ id: 1, level: 'success', title: 'Saved', message: 'Runbook saved' }]}
        onDismiss={vi.fn()}
      />,
    )

    const toast = screen.getByRole('status')
    expect(toast.getAttribute('aria-live')).toBe('polite')
    expect(toast.textContent).toContain('Runbook saved')
  })
})
