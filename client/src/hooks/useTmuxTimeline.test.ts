// @vitest-environment jsdom
import { createElement } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useTmuxTimeline } from './useTmuxTimeline'
import type { TimelineResponse } from '@/types'
import type { ApiFunction } from './tmuxTypes'
import type { ReactNode } from 'react'

function buildTimelineResponse(session: string): TimelineResponse {
  return {
    events: [
      {
        id: 1,
        session,
        windowIndex: 0,
        paneId: '%1',
        eventType: 'exec',
        severity: 'info',
        command: 'ls',
        cwd: '/tmp',
        durationMs: 0,
        summary: `event-${session}`,
        details: '',
        marker: '',
        metadata: null,
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
    hasMore: false,
  }
}

describe('useTmuxTimeline', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
          gcTime: 0,
        },
      },
    })
  })

  afterEach(() => {
    queryClient.clear()
  })

  function wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }

  it('reloads timeline when active session changes under active filter', async () => {
    const apiMock = vi.fn((path: string) => {
      const url = new URL(path, 'http://localhost')
      const session = url.searchParams.get('session') ?? ''
      return Promise.resolve(buildTimelineResponse(session))
    })
    const api = apiMock as unknown as ApiFunction

    let activeSession = 'alpha'
    const { result, rerender } = renderHook(
      () =>
        useTmuxTimeline({
          api,
          activeSession,
        }),
      { wrapper },
    )

    act(() => {
      result.current.setTimelineOpen(true)
    })

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalled()
    })

    await waitFor(() => {
      expect(
        apiMock.mock.calls.some((call) =>
          String(call[0]).includes('session=alpha'),
        ),
      ).toBe(true)
    })

    activeSession = 'beta'
    rerender()

    await waitFor(() => {
      expect(
        apiMock.mock.calls.some((call) =>
          String(call[0]).includes('session=beta'),
        ),
      ).toBe(true)
    })
  })
})
