// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ConnectionIssueBanner } from './ConnectionIssueBanner'

const { writeClipboardTextMock } = vi.hoisted(() => ({
  writeClipboardTextMock: vi.fn(),
}))

vi.mock('@/lib/clipboardProvider', () => ({
  writeClipboardText: writeClipboardTextMock,
}))

describe('ConnectionIssueBanner', () => {
  it('shows the cause, config path, exact configuration, and recovery actions', () => {
    const onRetry = vi.fn()
    const configuration = '[server]\ntrusted_proxies = ["192.0.2.10"]'
    render(
      <ConnectionIssueBanner
        issue={{
          code: 'UNTRUSTED_PROXY',
          title: 'HTTPS proxy is not trusted',
          message: 'HTTPS proxy "192.0.2.10" is not trusted.',
          configPath: '/root/.sentinel/config.toml',
          configuration,
        }}
        checking={false}
        onRetry={onRetry}
      />,
    )

    expect(screen.getByRole('alert')).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'HTTPS proxy is not trusted' })).toBeTruthy()
    expect(screen.getByText(/Edit \/root\/\.sentinel\/config\.toml/)).toBeTruthy()
    expect(document.querySelector('code')?.textContent).toBe(configuration)

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    expect(writeClipboardTextMock).toHaveBeenCalledWith(configuration)
    expect(screen.getByRole('button', { name: 'Copied' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
