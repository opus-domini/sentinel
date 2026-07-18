// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import MCPSettingsPanel from './MCPSettingsPanel'

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
}))

vi.mock('@/hooks/useTmuxApi', () => ({
  useTmuxApi: () => mocks.api,
}))

describe('MCPSettingsPanel', () => {
  afterEach(cleanup)

  beforeEach(() => {
    mocks.api.mockReset()
    mocks.api.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/ops/settings/mcp' && init?.method === 'PATCH') {
        return Promise.resolve({ enabled: true, tokenConfigured: true, endpoint: '/mcp' })
      }
      return Promise.resolve({ enabled: false, tokenConfigured: true, endpoint: '/mcp' })
    })
  })

  it('shows client hints and enables the live endpoint', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <MCPSettingsPanel hostname="Azdrix.LAN" />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Remote agent access')).toBeTruthy()
    expect(screen.getByText(`${window.location.origin}/mcp`)).toBeTruthy()
    expect(screen.getByText(/codex mcp add sentinel-azdrix-lan/).textContent).toContain(
      '--bearer-token-env-var SENTINEL_TOKEN',
    )
    expect(container.querySelector('pre')?.className).toContain('max-w-full')

    fireEvent.click(screen.getByLabelText('Enable MCP'))
    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith('/api/ops/settings/mcp', {
        method: 'PATCH',
        body: JSON.stringify({ enabled: true }),
      }),
    )
    expect(await screen.findByText('Available')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Claude' }))
    expect(screen.getByText(/claude mcp add-json/).textContent).toContain(
      'claude mcp add-json --scope user sentinel-azdrix-lan',
    )
    fireEvent.click(screen.getByRole('button', { name: 'mcpServers' }))
    expect(screen.getByText(/"mcpServers"/)).toBeTruthy()
    expect(screen.getByText(/"sentinel-azdrix-lan"/)).toBeTruthy()
  })

  it('blocks enable when server.token is missing', async () => {
    mocks.api.mockResolvedValue({ enabled: false, tokenConfigured: false, endpoint: '/mcp' })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <MCPSettingsPanel hostname="azdrix" />
      </QueryClientProvider>,
    )

    const tokenCode = await screen.findByText('server.token')
    expect(tokenCode.parentElement?.textContent).toContain(
      'Configure server.token before enabling MCP',
    )
    expect((screen.getByLabelText('Enable MCP') as HTMLInputElement).disabled).toBe(true)
  })
})
