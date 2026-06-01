// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ErrorBoundary } from './ErrorBoundary'

function ThrowingChild(): never {
  throw new Error('boom')
}

describe('ErrorBoundary', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('shows retry, tmux, and reload recovery actions', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <ThrowingChild />
      </ErrorBoundary>,
    )

    expect(screen.getByText('Something went wrong')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Try again' })).not.toBeNull()
    expect(screen.getByRole('link', { name: 'Go to tmux' }).getAttribute('href')).toBe('/tmux')
    expect(screen.getByRole('button', { name: 'Reload page' })).not.toBeNull()
  })

  it('keeps Try again wired to re-render children', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    let shouldThrow = true

    function RecoveringChild() {
      if (shouldThrow) throw new Error('boom')
      return <div>Recovered</div>
    }

    render(
      <ErrorBoundary>
        <RecoveringChild />
      </ErrorBoundary>,
    )

    shouldThrow = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

    expect(screen.getByText('Recovered')).not.toBeNull()
  })
})
