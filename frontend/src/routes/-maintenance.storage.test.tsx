// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

import type { StorageStatsResponse } from '@/types'
import {
  StorageMaintenanceError,
  StorageMaintenanceLoading,
  StorageMaintenancePage,
} from './maintenance.storage'

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({ hostname: 'storage-test' }),
}))

vi.mock('@/hooks/useTmuxApi', () => ({
  useTmuxApi: () => mocks.api,
}))

const stats: StorageStatsResponse = {
  databaseBytes: 8192,
  walBytes: 1024,
  shmBytes: 512,
  totalBytes: 9728,
  collectedAt: '2026-07-27T12:00:00Z',
  resources: [
    {
      resource: 'activity-journal',
      label: 'Activity journal',
      totalRows: 1,
      flushableRows: 1,
      protectedRows: 0,
      approxBytes: 256,
    },
    {
      resource: 'ops-jobs',
      label: 'Ops runbook jobs',
      totalRows: 5,
      flushableRows: 2,
      protectedRows: 3,
      approxBytes: 1024,
    },
  ],
}

describe('Storage maintenance route', () => {
  afterEach(cleanup)

  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    mocks.api.mockReset()
  })

  it('renders named loading and retryable error states', () => {
    const retry = vi.fn()
    const { rerender } = render(<StorageMaintenanceLoading />)
    expect(screen.getByLabelText('Loading storage maintenance')).toBeTruthy()

    rerender(<StorageMaintenanceError message="database busy" onRetry={retry} />)
    expect(screen.getByText('Storage data is unavailable')).toBeTruthy()
    expect(screen.getByText('database busy')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('shows impact, supports cancellation, and reports a successful atomic cleanup', async () => {
    mocks.api.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/ops/storage/stats') return Promise.resolve(stats)
      if (path === '/api/ops/storage/flush' && init?.method === 'POST') {
        return Promise.resolve({
          results: [
            { resource: 'activity-journal', removedRows: 1, protectedRows: 0 },
            { resource: 'ops-jobs', removedRows: 2, protectedRows: 3 },
          ],
          flushedAt: '2026-07-27T12:01:00Z',
        })
      }
      return Promise.reject(new Error(`unexpected request: ${path}`))
    })
    renderPage()

    expect(await screen.findByText('Activity journal')).toBeTruthy()
    expect(screen.getByText('Ops runbook jobs')).toBeTruthy()
    expect(
      screen.getByText('Queued, running, and approval-waiting jobs stay intact.', {
        exact: false,
      }),
    ).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible data' }))
    expect(screen.getByRole('alertdialog')).toBeTruthy()
    expect(screen.getByText(/permanently removes 3 eligible rows/)).toBeTruthy()
    expect(screen.getByText(/3 active rows will be preserved/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(mocks.api).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible data' }))
    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible rows' }))

    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith('/api/ops/storage/flush', {
        method: 'POST',
        body: '{"resource":"all"}',
      }),
    )
    expect((await screen.findByRole('status')).textContent).toContain(
      '3 eligible rows removed. 3 active rows preserved.',
    )
  })

  it('renders the empty state when nothing is eligible', async () => {
    mocks.api.mockResolvedValue({
      ...stats,
      resources: stats.resources.map((resource) => ({
        ...resource,
        totalRows: resource.protectedRows,
        flushableRows: 0,
      })),
    })
    renderPage()

    expect(await screen.findByText('No historical rows are eligible for removal.')).toBeTruthy()
    expect(
      (screen.getByRole('button', { name: 'Clear eligible data' }) as HTMLButtonElement).disabled,
    ).toBe(true)
  })

  it('sends a targeted resource after explicit selection', async () => {
    mocks.api.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/ops/storage/stats') return Promise.resolve(stats)
      if (path === '/api/ops/storage/flush' && init?.method === 'POST') {
        return Promise.resolve({
          results: [{ resource: 'ops-jobs', removedRows: 2, protectedRows: 3 }],
          flushedAt: '2026-07-27T12:01:00Z',
        })
      }
      return Promise.reject(new Error(`unexpected request: ${path}`))
    })
    renderPage()

    await screen.findByText('Activity journal')
    fireEvent.click(screen.getByRole('combobox', { name: 'Storage resource' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Ops runbook jobs' }))
    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible data' }))

    expect(screen.getByText(/2 eligible rows from Ops runbook jobs/)).toBeTruthy()
    expect(screen.getByText(/3 active rows will be preserved/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible rows' }))

    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith('/api/ops/storage/flush', {
        method: 'POST',
        body: '{"resource":"ops-jobs"}',
      }),
    )
  })

  it('surfaces cleanup failure without losing the loaded resource data', async () => {
    mocks.api.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/ops/storage/stats') return Promise.resolve(stats)
      if (path === '/api/ops/storage/flush' && init?.method === 'POST') {
        return Promise.reject(new Error('database transaction rolled back'))
      }
      return Promise.reject(new Error(`unexpected request: ${path}`))
    })
    renderPage()

    await screen.findByText('Activity journal')
    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible data' }))
    fireEvent.click(screen.getByRole('button', { name: 'Clear eligible rows' }))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'database transaction rolled back',
    )
    expect(screen.getByText('Ops runbook jobs')).toBeTruthy()
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <StorageMaintenancePage />
    </QueryClientProvider>,
  )
}
