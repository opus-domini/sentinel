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
}: {
  api: ServiceLogsAPI
  fetchKey?: number
  target?: OpsBrowsedService
}) {
  return render(
    <TooltipProvider>
      <ServiceLogsSheet
        open
        onOpenChange={() => {}}
        fetchKey={fetchKey}
        service={target}
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
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      rafCallbacks.push(callback)
      return rafCallbacks.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
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

  it('batches streamed lines into a single animation-frame flush', async () => {
    const { api, spy } = createAPI()

    renderSheet({ api })

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    const streamOptions = await waitFor(() => {
      const enabledCall = useLogStreamMock.mock.calls.find((call) => call[0].enabled)
      if (!enabledCall) throw new Error('stream not enabled')
      return enabledCall[0]
    })

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
