// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import RestartPendingNotice from './RestartPendingNotice'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  writeClipboardText: vi.fn(),
}))

vi.mock('@/lib/clipboardProvider', () => ({
  writeClipboardText: mocks.writeClipboardText,
}))

afterEach(() => {
  cleanup()
  mocks.writeClipboardText.mockReset()
})

describe('RestartPendingNotice', () => {
  it('shows changed keys and scope and confirms copy only after clipboard success', async () => {
    let resolveCopy: ((value: boolean) => void) | undefined
    mocks.writeClipboardText.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveCopy = resolve
      }),
    )
    const snapshot = createSettingsSnapshot().settings

    render(
      <RestartPendingNotice
        deployment={{ ...snapshot.deployment, scope: 'user', runtimeMode: 'service' }}
        restart={{
          ...snapshot.restart,
          required: true,
          changedKeys: ['log.level', 'runbooks.max_concurrent'],
          command: 'sentinel service restart --scope user',
        }}
      />,
    )

    expect(screen.getByText('user')).toBeTruthy()
    expect(screen.getByText(/log.level · runbooks.max_concurrent/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    expect(screen.queryByRole('button', { name: 'Copied' })).toBeNull()

    resolveCopy?.(true)
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeTruthy()
  })
})
