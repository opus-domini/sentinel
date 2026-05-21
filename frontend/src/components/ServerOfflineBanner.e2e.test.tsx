// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { ServerOfflineBanner } from '@/components/ServerOfflineBanner'
import { useServerStatus } from '@/hooks/useServerStatus'
import { getServerOffline, queryClient, retryServer } from '@/lib/queryClient'

function TestHarness() {
  const { offline, retry } = useServerStatus()
  if (!offline) return <p>online</p>
  return <ServerOfflineBanner onRetry={retry} />
}

function renderWithClient() {
  return render(
    <QueryClientProvider client={queryClient}>
      <TestHarness />
    </QueryClientProvider>,
  )
}

function triggerNetworkError() {
  const cache = queryClient.getQueryCache()
  const onError = cache.config.onError as ((error: Error) => void) | undefined
  onError?.(new TypeError('Failed to fetch'))
}

function triggerSuccess() {
  const cache = queryClient.getQueryCache()
  const onSuccess = cache.config.onSuccess as
    | ((data: unknown) => void)
    | undefined
  onSuccess?.(undefined)
}

describe('ServerOfflineBanner', () => {
  afterEach(() => {
    cleanup()
    retryServer()
    queryClient.clear()
  })

  it('shows banner when a network error occurs', () => {
    renderWithClient()
    expect(screen.getByText('online')).toBeTruthy()

    act(() => {
      triggerNetworkError()
    })

    expect(screen.getByText('Server unreachable')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })

  it('dismisses banner when retry is clicked', () => {
    renderWithClient()

    act(() => {
      triggerNetworkError()
    })

    expect(screen.getByText('Server unreachable')).toBeTruthy()

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    })

    expect(screen.getByText('online')).toBeTruthy()
    expect(getServerOffline()).toBe(false)
  })

  it('auto-dismisses when a query succeeds', () => {
    renderWithClient()

    act(() => {
      triggerNetworkError()
    })

    expect(screen.getByText('Server unreachable')).toBeTruthy()

    act(() => {
      triggerSuccess()
    })

    expect(screen.getByText('online')).toBeTruthy()
  })

  it('ignores non-network errors', () => {
    renderWithClient()

    act(() => {
      const cache = queryClient.getQueryCache()
      const onError = cache.config.onError as
        | ((error: Error) => void)
        | undefined
      onError?.(new Error('HTTP 500'))
    })

    expect(screen.getByText('online')).toBeTruthy()
    expect(getServerOffline()).toBe(false)
  })
})
