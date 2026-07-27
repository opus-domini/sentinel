// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NowError, NowLoading } from './index'

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
}))

afterEach(cleanup)

describe('Now route states', () => {
  it('renders a named loading state', () => {
    render(<NowLoading />)

    expect(screen.getByLabelText('Loading Now')).toBeTruthy()
  })

  it('renders an actionable error state', () => {
    const onRetry = vi.fn()
    render(<NowError message="snapshot unavailable" onRetry={onRetry} />)

    expect(screen.getByText('Now is unavailable')).toBeTruthy()
    expect(screen.getByText('snapshot unavailable')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })
})
