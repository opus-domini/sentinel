// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import type * as SettingsAPI from '@/api/settings'
import type * as SettingsRestart from '@/lib/settingsRestart'

import RestartPendingNotice from './RestartPendingNotice'
import { SETTINGS_QUERY_KEY } from '@/hooks/useSettings'
import { createSettingsSnapshot } from '@/test/settings'

const mocks = vi.hoisted(() => ({
  writeClipboardText: vi.fn(),
  restartSettings: vi.fn(),
  waitForSettingsRestart: vi.fn(),
  waitForRestartTarget: vi.fn(),
  navigateToRestartTarget: vi.fn(),
  reloadForAuthentication: vi.fn(),
  pushToast: vi.fn(),
}))

vi.mock('@/lib/clipboardProvider', () => ({
  writeClipboardText: mocks.writeClipboardText,
}))

vi.mock('@/api/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof SettingsAPI>()),
  restartSettings: mocks.restartSettings,
}))

vi.mock('@/lib/settingsRestart', async (importOriginal) => ({
  ...(await importOriginal<typeof SettingsRestart>()),
  waitForSettingsRestart: mocks.waitForSettingsRestart,
  waitForRestartTarget: mocks.waitForRestartTarget,
  navigateToRestartTarget: mocks.navigateToRestartTarget,
  reloadForAuthentication: mocks.reloadForAuthentication,
}))

vi.mock('@/contexts/ToastContext', () => ({
  useToastContext: () => ({ pushToast: mocks.pushToast }),
}))

afterEach(() => {
  cleanup()
})

beforeEach(() => {
  mocks.writeClipboardText.mockReset()
  mocks.restartSettings.mockReset()
  mocks.waitForSettingsRestart.mockReset()
  mocks.waitForRestartTarget.mockReset()
  mocks.navigateToRestartTarget.mockReset()
  mocks.reloadForAuthentication.mockReset()
  mocks.pushToast.mockReset()
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

    renderNotice(
      <RestartPendingNotice
        revision={snapshot.revision}
        deployment={{ ...snapshot.deployment, scope: 'user', runtimeMode: 'service' }}
        restart={{
          ...snapshot.restart,
          required: true,
          available: true,
          changedKeys: ['log.level', 'runbooks.max_concurrent'],
          command: 'sentinel service restart --scope user',
        }}
      />,
    )

    expect(screen.getByText('user')).toBeTruthy()
    expect(screen.getByText(/log.level · runbooks.max_concurrent/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy command' }))
    expect(screen.queryByRole('button', { name: 'Copied' })).toBeNull()

    resolveCopy?.(true)
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeTruthy()
  })

  it('confirms, restarts, and adopts the applied settings snapshot', async () => {
    const onRestartComplete = vi.fn()
    const pending = createSettingsSnapshot({ revision: 'a'.repeat(64) })
    pending.settings.deployment = {
      scope: 'user',
      runtimeMode: 'service',
      configPath: '/home/test/.sentinel/config.toml',
    }
    pending.settings.restart = {
      required: true,
      available: true,
      changedKeys: ['watchtower.tick_interval'],
      backupPath: '/home/test/.sentinel/config.toml.bak',
      command: 'sentinel service restart --scope user',
      instruction: 'Restart Sentinel to activate the saved configuration.',
    }
    const complete = createSettingsSnapshot({ revision: 'b'.repeat(64) })
    mocks.restartSettings.mockResolvedValue({
      status: 'accepted',
      scope: 'user',
      changedKeys: pending.settings.restart.changedKeys,
    })
    mocks.waitForSettingsRestart.mockResolvedValue({
      status: 'complete',
      snapshot: complete,
    })
    const { queryClient } = renderNotice(
      <RestartPendingNotice
        revision={pending.settings.revision}
        deployment={pending.settings.deployment}
        restart={pending.settings.restart}
        onRestartComplete={onRestartComplete}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Restart Sentinel' }))
    const dialog = screen.getByRole('alertdialog')
    expect(dialog.textContent).toContain('watchtower.tick_interval')
    expect(dialog.textContent).toContain('Existing tmux sessions are not targeted')

    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart Sentinel' }))

    await waitFor(() =>
      expect(mocks.restartSettings).toHaveBeenCalledWith(pending.settings.revision),
    )
    await waitFor(() => expect(queryClient.getQueryData(SETTINGS_QUERY_KEY)).toEqual(complete))
    expect(onRestartComplete).toHaveBeenCalledOnce()
    expect(mocks.pushToast).toHaveBeenCalledWith({
      level: 'success',
      title: 'Sentinel restarted',
      message: 'Saved settings are active and the service is responding again.',
    })
  })

  it('keeps manual recovery without offering a false restart action', () => {
    const snapshot = createSettingsSnapshot().settings
    renderNotice(
      <RestartPendingNotice
        revision={snapshot.revision}
        deployment={snapshot.deployment}
        restart={{
          ...snapshot.restart,
          required: true,
          command: 'supervisorctl restart sentinel',
        }}
      />,
    )

    expect(screen.queryByRole('button', { name: 'Restart Sentinel' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Copy command' })).toBeTruthy()
  })
})

function renderNotice(node: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    queryClient,
    ...render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>),
  }
}
