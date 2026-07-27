// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import type { OpsServiceInspect, OpsServiceStatusResponse } from '@/types'
import { ServiceStatusDialog } from './ServiceStatusDialog'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
    ...rest
  }: {
    children: ReactNode
    to: string
    search: Record<string, string>
  }) => (
    <a href={`${to}?${new URLSearchParams(search)}`} {...rest}>
      {children}
    </a>
  ),
}))

afterEach(cleanup)

describe('ServiceStatusDialog', () => {
  it('presents condition and owner links before raw details, then hands off to logs', () => {
    const onViewLogs = vi.fn()
    const data = {
      service: {
        name: 'sentinel',
        displayName: 'Sentinel',
        trackingMode: 'builtin',
        manager: 'systemd',
        scope: 'user',
        unit: 'sentinel.service',
        exists: true,
        enabledState: 'enabled',
        activeState: 'failed',
        updatedAt: '2026-07-27T12:00:00Z',
      },
      summary: 'failed',
      condition: {
        activeState: 'failed',
        subState: 'failed',
        result: 'exit-code',
        exitCode: 1,
        exitStatus: 42,
        transitionedAt: '2026-07-27T11:59:00Z',
      },
      observedAt: '2026-07-27T12:00:00Z',
      properties: { FragmentPath: '/unit/sentinel.service' },
      output: 'raw manager output',
    } satisfies OpsServiceInspect
    const context = {
      runbook: { id: 'runbook-1', name: 'Recover Sentinel' },
      latestRun: {
        id: 'job-1',
        runbookName: 'Recover Sentinel',
        status: 'failed',
      },
    } as OpsServiceStatusResponse['context']

    render(
      <ServiceStatusDialog
        open
        onOpenChange={vi.fn()}
        loading={false}
        error=""
        data={data}
        context={context}
        onViewLogs={onViewLogs}
      />,
    )

    expect(screen.getByText('Current condition')).toBeTruthy()
    expect(screen.getByText('exit-code')).toBeTruthy()
    expect(screen.getByText('42')).toBeTruthy()
    expect(screen.getByRole('link', { name: /Procedure/ }).getAttribute('href')).toBe(
      '/runbooks?runbook=runbook-1',
    )
    expect(screen.getByRole('link', { name: /Latest execution/ }).getAttribute('href')).toBe(
      '/runbooks?job=job-1',
    )
    expect(
      screen.getByText('Current condition').compareDocumentPosition(screen.getByText('Properties')),
    ).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
    fireEvent.click(screen.getByRole('button', { name: 'View logs' }))

    expect(onViewLogs).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: /restart|start|stop/i })).toBeNull()
  })
})
