// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NowError, NowLoading, startNowServiceProcedure } from './index'

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

  it('hands a started Now procedure to canonical job navigation', async () => {
    const job = {
      id: 'job-1',
      runbookId: 'runbook-1',
      runbookName: 'Recover service',
      status: 'queued',
      totalSteps: 1,
      completedSteps: 0,
      currentStep: 'Recover',
      error: '',
      stepResults: [],
      createdAt: '2026-07-27T12:00:00Z',
    }
    const api = vi.fn().mockResolvedValue({ job })
    const onStarted = vi.fn()

    await expect(
      startNowServiceProcedure(api, 'api service', { MODE: 'safe' }, onStarted),
    ).resolves.toEqual(job)

    expect(api).toHaveBeenCalledWith('/api/now/services/api%20service/runbook', {
      method: 'POST',
      body: '{"parameters":{"MODE":"safe"}}',
    })
    expect(onStarted).toHaveBeenCalledWith(job)
  })
})
