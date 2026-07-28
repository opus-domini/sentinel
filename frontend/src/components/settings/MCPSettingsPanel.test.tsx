// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import MCPSettingsPanel from './MCPSettingsPanel'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  writeClipboardText: vi.fn(),
}))

vi.mock('@/lib/clipboardProvider', () => ({
  writeClipboardText: mocks.writeClipboardText,
}))

describe('MCPSettingsPanel', () => {
  afterEach(cleanup)

  beforeEach(() => {
    mocks.writeClipboardText.mockReset()
    mocks.writeClipboardText.mockResolvedValue(true)
  })

  it('shows client hints and exposes the desired enabled state', () => {
    const onEnabledChange = vi.fn()
    renderPanel({ hostname: 'Azdrix.LAN', onEnabledChange })

    expect(screen.getByText('Remote agent access')).toBeTruthy()
    expect(screen.getByText(`${window.location.origin}/mcp`)).toBeTruthy()
    expect(screen.getByText(/codex mcp add sentinel-azdrix-lan/).textContent).toContain(
      '--bearer-token-env-var SENTINEL_TOKEN',
    )

    fireEvent.click(screen.getByLabelText('Enable MCP'))
    expect(onEnabledChange).toHaveBeenCalledWith(true)

    fireEvent.click(screen.getByRole('button', { name: 'Claude' }))
    expect(screen.getByText(/claude mcp add-json/).textContent).toContain(
      'claude mcp add-json --scope user sentinel-azdrix-lan',
    )
    fireEvent.click(screen.getByRole('button', { name: 'mcpServers' }))
    expect(screen.getByText(/"mcpServers"/)).toBeTruthy()
  })

  it('blocks enable without a configured or staged replacement token', () => {
    renderPanel({
      settings: createSettingsSnapshot({
        tokenConfigured: false,
        runtimeTokenConfigured: false,
      }).settings.integrations.mcp,
    })

    expect(screen.getByText(/Replace the shared token before enabling MCP/)).toBeTruthy()
    expect((screen.getByLabelText('Enable MCP') as HTMLButtonElement).disabled).toBe(true)
  })

  it('allows desired enable with a staged replacement and reports pending restart', () => {
    const onEnabledChange = vi.fn()
    renderPanel({
      settings: createSettingsSnapshot({
        tokenConfigured: false,
        runtimeTokenConfigured: false,
      }).settings.integrations.mcp,
      enabled: true,
      tokenIntent: 'replace',
      tokenValue: 'new-secret',
      onEnabledChange,
    })

    expect(screen.getByText('Pending restart')).toBeTruthy()
    expect(screen.getByText(/started without the saved token/)).toBeTruthy()
    expect((screen.getByLabelText('Enable MCP') as HTMLButtonElement).disabled).toBe(false)
    expect(document.body.textContent).not.toContain('new-secret')
  })

  it('confirms clipboard success only after the write resolves', async () => {
    let resolveClipboard: ((value: boolean) => void) | undefined
    mocks.writeClipboardText.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveClipboard = resolve
      }),
    )
    renderPanel()

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0])
    expect(screen.queryByText('Copied')).toBeNull()
    resolveClipboard?.(true)
    expect(await screen.findByText('Copied')).toBeTruthy()
  })

  it('reports clipboard denial without false success', async () => {
    mocks.writeClipboardText.mockResolvedValue(false)
    renderPanel()

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0])
    expect(await screen.findByText(/Clipboard permission was denied/)).toBeTruthy()
    expect(screen.queryByText('Copied')).toBeNull()
  })
})

function renderPanel(overrides: Partial<React.ComponentProps<typeof MCPSettingsPanel>> = {}) {
  const settings = createSettingsSnapshot().settings.integrations.mcp
  return render(
    <MCPSettingsPanel
      hostname="azdrix"
      settings={settings}
      enabled={false}
      tokenIntent="keep"
      tokenValue=""
      saving={false}
      onEnabledChange={vi.fn()}
      onTokenIntentChange={vi.fn()}
      onTokenValueChange={vi.fn()}
      {...overrides}
    />,
  )
}
