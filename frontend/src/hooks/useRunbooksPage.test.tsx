// @vitest-environment jsdom
import { createElement } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useRunbooksPage } from './useRunbooksPage'
import { ApiError } from './useTmuxApi'
import type { ReactNode } from 'react'
import type { OpsRunbook, OpsRunbookRun, OpsRunbooksResponse } from '@/types'

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  pushToast: vi.fn(),
  opsHandler: undefined as ((message: unknown) => void) | undefined,
}))

vi.mock('@/hooks/useTmuxApi', () => ({
  ApiError: class MockApiError extends Error {
    status: number
    code: string
    details: unknown

    constructor(message: string, status: number, code = '', details?: unknown) {
      super(message)
      this.status = status
      this.code = code
      this.details = details
    }
  },
  useTmuxApi: () => mocks.api,
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: mocks.pushToast }),
}))

vi.mock('@/hooks/useOpsEvents', () => ({
  useOpsEvents: (handler: (message: unknown) => void) => {
    mocks.opsHandler = handler
    return 'connected'
  },
}))

function makeRunbook(overrides: Partial<OpsRunbook> = {}): OpsRunbook {
  return {
    id: 'runbook-1',
    name: 'Approval flow',
    description: '',
    enabled: true,
    steps: [
      { type: 'approval', title: 'Approve', description: 'Continue?' },
      { type: 'run', title: 'Finish', command: 'true' },
    ],
    createdAt: '2026-05-01T10:00:00Z',
    updatedAt: '2026-05-01T10:00:00Z',
    ...overrides,
  }
}

function makeJob(status: string, overrides: Partial<OpsRunbookRun> = {}) {
  return {
    id: 'job-1',
    runbookId: 'runbook-1',
    runbookName: 'Approval flow',
    status,
    totalSteps: 2,
    completedSteps: status === 'succeeded' ? 2 : 1,
    currentStep: status === 'succeeded' ? 'Finish' : 'Approve',
    error: '',
    stepResults: [],
    createdAt: '2026-05-01T10:00:00Z',
    startedAt: '2026-05-01T10:00:01Z',
    finishedAt: status === 'succeeded' ? '2026-05-01T10:00:02Z' : '',
    ...overrides,
  }
}

function makeResponse(job: OpsRunbookRun): OpsRunbooksResponse {
  return {
    runbooks: [makeRunbook()],
    jobs: [job],
    schedules: [],
  }
}

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('useRunbooksPage', () => {
  beforeEach(() => {
    mocks.api.mockReset()
    mocks.pushToast.mockReset()
    mocks.opsHandler = undefined
    vi.useRealTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('refreshes an approved job until the resumed run settles', async () => {
    const waitingJob = makeJob('waiting_approval')
    const runningJob = makeJob('running', { currentStep: 'Finish' })
    const succeededJob = makeJob('succeeded')
    const getResponses = [
      makeResponse(waitingJob),
      makeResponse(runningJob),
      makeResponse(succeededJob),
    ]
    let getCount = 0

    mocks.api.mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/ops/runbooks') {
        const response = getResponses[Math.min(getCount, getResponses.length - 1)]
        getCount += 1
        return Promise.resolve(response)
      }
      if (url === '/api/ops/runs/job-1/approve' && init?.method === 'POST') {
        return Promise.resolve({ job: waitingJob })
      }
      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useRunbooksPage(), {
      wrapper: makeWrapper(queryClient),
    })

    await waitFor(() => {
      expect(result.current.jobs[0]?.status).toBe('waiting_approval')
    })

    await act(async () => {
      await result.current.approveJob('job-1')
    })

    await waitFor(() => {
      expect(result.current.jobs[0]?.status).toBe('running')
    })

    await waitFor(() => {
      expect(result.current.jobs[0]?.status).toBe('succeeded')
    })
    expect(getCount).toBe(3)
    expect(mocks.pushToast).toHaveBeenCalledWith(
      expect.objectContaining({
        level: 'success',
        title: 'Approval accepted',
      }),
    )
  })

  it('deduplicates polling when the same job is tracked more than once', async () => {
    const waitingJob = makeJob('waiting_approval')
    const runningJob = makeJob('running', { currentStep: 'Finish' })
    let getCount = 0

    mocks.api.mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/ops/runbooks') {
        getCount += 1
        return Promise.resolve(makeResponse(getCount === 1 ? waitingJob : runningJob))
      }
      if (url === '/api/ops/runs/job-1/approve' && init?.method === 'POST') {
        return Promise.resolve({ job: runningJob })
      }
      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useRunbooksPage(), {
      wrapper: makeWrapper(queryClient),
    })

    await waitFor(() => {
      expect(result.current.jobs[0]?.status).toBe('waiting_approval')
    })

    vi.useFakeTimers()

    await act(async () => {
      await result.current.approveJob('job-1')
      await result.current.approveJob('job-1')
    })

    expect(getCount).toBe(3)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250)
    })

    expect(getCount).toBe(4)
  })

  it('cancels scheduled polling when a websocket job update is terminal', async () => {
    const waitingJob = makeJob('waiting_approval')
    const runningJob = makeJob('running', { currentStep: 'Finish' })
    const succeededJob = makeJob('succeeded')
    let getCount = 0

    mocks.api.mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/ops/runbooks') {
        getCount += 1
        return Promise.resolve(makeResponse(getCount === 1 ? waitingJob : runningJob))
      }
      if (url === '/api/ops/runs/job-1/approve' && init?.method === 'POST') {
        return Promise.resolve({ job: runningJob })
      }
      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useRunbooksPage(), {
      wrapper: makeWrapper(queryClient),
    })

    await waitFor(() => {
      expect(result.current.jobs[0]?.status).toBe('waiting_approval')
    })

    vi.useFakeTimers()

    await act(async () => {
      await result.current.approveJob('job-1')
    })
    expect(getCount).toBe(2)

    act(() => {
      mocks.opsHandler?.({ type: 'ops.job.updated', payload: { job: succeededJob } })
    })

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250)
    })

    expect(getCount).toBe(2)
    expect(result.current.jobs[0]?.status).toBe('succeeded')
  })

  it('loads and updates a focused receipt independently from the definition list', async () => {
    const focused = makeJob('running', {
      id: 'orphan-job',
      runbookId: 'deleted-runbook',
      runbookName: 'Deleted definition',
    })
    const updated = makeJob('succeeded', {
      ...focused,
      status: 'succeeded',
      completedSteps: 2,
      finishedAt: '2026-05-01T10:00:02Z',
    })

    mocks.api.mockImplementation((url: string) => {
      if (url === '/api/ops/runbooks') {
        return Promise.resolve({ runbooks: [], jobs: [], schedules: [] })
      }
      if (url === '/api/ops/jobs/orphan-job') {
        return Promise.resolve({ job: focused })
      }
      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useRunbooksPage('orphan-job'), {
      wrapper: makeWrapper(queryClient),
    })

    await waitFor(() => {
      expect(result.current.focusedJob?.id).toBe('orphan-job')
      expect(result.current.jobs).toEqual([focused])
    })

    act(() => {
      mocks.opsHandler?.({ type: 'ops.job.updated', payload: { job: updated } })
    })

    await waitFor(() => {
      expect(result.current.focusedJob?.status).toBe('succeeded')
    })
  })

  it('returns a started receipt and keeps run context open on a typed target conflict', async () => {
    const queued = makeJob('queued')
    let startCount = 0

    mocks.api.mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/ops/runbooks') return Promise.resolve(makeResponse(queued))
      if (url === '/api/ops/runbooks/runbook-1/run' && init?.method === 'POST') {
        startCount += 1
        if (startCount === 1) return Promise.resolve({ job: queued })
        return Promise.reject(
          new ApiError(
            'target service already has an active execution',
            409,
            'RUNBOOK_TARGET_BUSY',
          ),
        )
      }
      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useRunbooksPage(), {
      wrapper: makeWrapper(queryClient),
    })

    await waitFor(() => expect(result.current.runbooks).toHaveLength(1))

    act(() => result.current.startRun('runbook-1'))
    expect(result.current.runTarget?.id).toBe('runbook-1')

    await act(async () => {
      await expect(result.current.confirmRun({})).resolves.toEqual(queued)
    })
    expect(result.current.runTarget).toBeNull()

    act(() => result.current.startRun('runbook-1'))

    await act(async () => {
      await expect(result.current.confirmRun({})).resolves.toBeNull()
    })

    expect(result.current.runTarget?.id).toBe('runbook-1')
    expect(mocks.pushToast).toHaveBeenLastCalledWith({
      level: 'error',
      title: 'Procedure already active',
      message: 'Review the active execution in this runbook before starting another.',
    })
  })
})
