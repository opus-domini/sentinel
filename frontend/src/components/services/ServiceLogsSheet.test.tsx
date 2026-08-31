// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import type { OpsBrowsedService } from '@/types'
import { ServiceLogsSheet } from './ServiceLogsSheet'

const { useLogStreamMock } = vi.hoisted(() => ({
  useLogStreamMock: vi.fn<
    (options: { enabled: boolean; onLine: (line: string) => void }) => string
  >(() => 'disconnected'),
}))

vi.mock('@/hooks/useLogStream', () => ({
  useLogStream: useLogStreamMock,
}))

type ServiceLogsAPI = <T>(url: string, init?: RequestInit) => Promise<T>

function service(partial: Partial<OpsBrowsedService> = {}): OpsBrowsedService {
  return {
    unit: 'app-gnome\\x2dkeyring.service',
    unitType: 'service',
    description: 'Certificate and Key Storage',
    activeState: 'inactive',
    enabledState: 'generated',
    manager: 'systemd',
    scope: 'user',
    tracked: false,
    ...partial,
  }
}

function createAPI() {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const spy = vi.fn()
  const api: ServiceLogsAPI = async <T,>(url: string, init?: RequestInit): Promise<T> => {
    spy(url, init)
    calls.push({ url, init })
    return { output: 'started\nready' } as T
  }

  return { api, calls, spy }
}

function renderSheet({
  api,
  fetchKey = 1,
  target = service(),
  since,
}: {
  api: ServiceLogsAPI
  fetchKey?: number
  target?: OpsBrowsedService
  since?: string
}) {
  return render(
    <TooltipProvider>
      <ServiceLogsSheet
        open
        onOpenChange={() => {}}
        fetchKey={fetchKey}
        service={target}
        since={since}
        authenticated
        tokenRequired={false}
        api={api}
      />
    </TooltipProvider>,
  )
}

describe('ServiceLogsSheet', () => {
  let rafCallbacks: Array<FrameRequestCallback> = []

  beforeEach(() => {
    rafCallbacks = []
    useLogStreamMock.mockClear()
    // LogViewer's bounded row window observes its scroll viewport, which jsdom
    // does not provide.
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      rafCallbacks.push(callback)
      return rafCallbacks.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows formatted systemd unit names but requests logs with the raw unit name', async () => {
    const { api, calls, spy } = createAPI()

    renderSheet({ api })

    expect(await screen.findByText('app-gnome-keyring.service')).toBeTruthy()

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    const { url } = calls[0]
    const parsed = new URL(url, 'http://sentinel.local')

    expect(parsed.pathname).toBe('/api/ops/services/unit/logs')
    expect(parsed.searchParams.get('unit')).toBe('app-gnome\\x2dkeyring.service')
    expect(parsed.searchParams.get('scope')).toBe('user')
    expect(parsed.searchParams.get('manager')).toBe('systemd')
    expect(parsed.searchParams.get('lines')).toBe('200')
  })

  it('requests the initial temporal slice and labels the live continuation', async () => {
    const { api, calls, spy } = createAPI()
    const since = '2026-07-27T11:59:00Z'

    renderSheet({
      api,
      since,
      target: service({ tracked: true, trackedName: 'sentinel' }),
    })

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    const parsed = new URL(calls[0].url, 'http://sentinel.local')
    expect(parsed.pathname).toBe('/api/ops/services/sentinel/logs')
    expect(parsed.searchParams.get('since')).toBe(since)
    expect(screen.getByText(/Initial slice since/).textContent).toContain(since)
    expect(screen.getByText(/Initial slice since/).textContent).toContain(
      'live lines continue as they arrive',
    )
  })

  it('refetches only when fetchKey changes for the same service', async () => {
    const { api, spy } = createAPI()
    const target = service()

    const { rerender } = renderSheet({ api, fetchKey: 1, target })

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))

    rerender(
      <TooltipProvider>
        <ServiceLogsSheet
          open
          onOpenChange={() => {}}
          fetchKey={1}
          service={target}
          authenticated
          tokenRequired={false}
          api={api}
        />
      </TooltipProvider>,
    )

    expect(spy).toHaveBeenCalledTimes(1)

    rerender(
      <TooltipProvider>
        <ServiceLogsSheet
          open
          onOpenChange={() => {}}
          fetchKey={2}
          service={target}
          authenticated
          tokenRequired={false}
          api={api}
        />
      </TooltipProvider>,
    )

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2))
  })

  it('drains buffered lines through the timeout fallback when no frame arrives', async () => {
    const { api, spy } = createAPI()

    renderSheet({ api })

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    const streamOptions = await waitFor(() => {
      const enabledCall = useLogStreamMock.mock.calls.find((call) => call[0].enabled)
      if (!enabledCall) throw new Error('stream not enabled')
      return enabledCall[0]
    })

    rafCallbacks.length = 0
    vi.mocked(window.requestAnimationFrame).mockClear()

    act(() => {
      streamOptions.onLine('background-line')
    })

    // requestAnimationFrame is stubbed to collect callbacks and never run them,
    // which is what a hidden tab does. The paired timeout has to drain instead.
    expect(screen.queryByText('background-line')).toBeNull()
    expect(await screen.findByText('background-line')).toBeTruthy()
    expect(rafCallbacks).toHaveLength(1)
  })

  it('batches streamed lines into a single animation-frame flush', async () => {
    const { api, spy } = createAPI()

    renderSheet({ api })

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    const streamOptions = await waitFor(() => {
      const enabledCall = useLogStreamMock.mock.calls.find((call) => call[0].enabled)
      if (!enabledCall) throw new Error('stream not enabled')
      return enabledCall[0]
    })

    // Radix primitives (Sheet, tooltips) may schedule their own frames during
    // mount. Discard those so the assertion measures only the batching of the
    // two streamed lines below, independent of machine load.
    rafCallbacks.length = 0
    vi.mocked(window.requestAnimationFrame).mockClear()

    act(() => {
      streamOptions.onLine('stream-one')
      streamOptions.onLine('stream-two')
    })

    expect(window.requestAnimationFrame).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('stream-one')).toBeNull()

    act(() => {
      rafCallbacks.shift()?.(performance.now())
    })

    expect(await screen.findByText('stream-one')).toBeTruthy()
    expect(await screen.findByText('stream-two')).toBeTruthy()
  })
})
