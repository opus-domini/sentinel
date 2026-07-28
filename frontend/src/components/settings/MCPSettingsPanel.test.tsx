// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import MCPSettingsPanel from './MCPSettingsPanel'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  patchSettings: vi.fn(),
  writeClipboardText: vi.fn(),
}))

vi.mock('@/api/settings', () => ({
  getSettings: mocks.getSettings,
  patchSettings: mocks.patchSettings,
}))

vi.mock('@/lib/clipboardProvider', () => ({
  writeClipboardText: mocks.writeClipboardText,
}))

describe('MCPSettingsPanel', () => {
  afterEach(cleanup)

  beforeEach(() => {
    mocks.getSettings.mockReset()
    mocks.patchSettings.mockReset()
    mocks.writeClipboardText.mockReset()
    mocks.getSettings.mockResolvedValue(createSettingsSnapshot())
    mocks.patchSettings.mockResolvedValue(createSettingsSnapshot({ mcpEnabled: true }))
    mocks.writeClipboardText.mockResolvedValue(true)
  })

  it('shows client hints and enables the live endpoint through the unified API', async () => {
    renderPanel('Azdrix.LAN')

    expect(await screen.findByText('Remote agent access')).toBeTruthy()
    expect(screen.getByText(`${window.location.origin}/mcp`)).toBeTruthy()
    expect(screen.getByText(/codex mcp add sentinel-azdrix-lan/).textContent).toContain(
      '--bearer-token-env-var SENTINEL_TOKEN',
    )

    fireEvent.click(screen.getByLabelText('Enable MCP'))
    await waitFor(() =>
      expect(mocks.patchSettings).toHaveBeenCalledWith(`"${'a'.repeat(64)}"`, {
        integrations: {
          mcp: { enabled: true },
        },
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
    mocks.getSettings.mockResolvedValue(createSettingsSnapshot({ tokenConfigured: false }))
    renderPanel('azdrix')

    const tokenCode = await screen.findByText('server.token')
    expect(tokenCode.parentElement?.textContent).toContain(
      'Configure server.token before enabling MCP',
    )
    expect((screen.getByLabelText('Enable MCP') as HTMLInputElement).disabled).toBe(true)
  })

  it('confirms clipboard success only after the write resolves', async () => {
    let resolveClipboard: ((value: boolean) => void) | undefined
    mocks.writeClipboardText.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveClipboard = resolve
      }),
    )
    renderPanel('azdrix')
    await screen.findByText('Remote agent access')

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0])
    expect(screen.queryByText('Copied')).toBeNull()
    resolveClipboard?.(true)
    expect(await screen.findByText('Copied')).toBeTruthy()
  })

  it('reports clipboard denial without showing false success', async () => {
    mocks.writeClipboardText.mockResolvedValue(false)
    renderPanel('azdrix')
    await screen.findByText('Remote agent access')

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0])
    expect(await screen.findByText(/Clipboard permission was denied/)).toBeTruthy()
    expect(screen.queryByText('Copied')).toBeNull()
  })
})

function renderPanel(hostname: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MCPSettingsPanel hostname={hostname} />
    </QueryClientProvider>,
  )
}
