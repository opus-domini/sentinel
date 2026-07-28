import { getSettings } from '@/api/settings'
import type { SettingsSnapshot } from '@/api/settings'
import { ApiError } from '@/hooks/useTmuxApi'

const restartPollAttempts = 40
const restartPollIntervalMs = 750

type RestartWaitOptions = {
  loadSettings?: () => Promise<SettingsSnapshot>
  wait?: (delayMs: number) => Promise<void>
  attempts?: number
}

export type SettingsRestartResult =
  | { status: 'complete'; snapshot: SettingsSnapshot }
  | { status: 'authentication-required' }

export class SettingsRestartTimeoutError extends Error {
  constructor() {
    super('Sentinel did not return within 30 seconds.')
    this.name = 'SettingsRestartTimeoutError'
  }
}

export async function waitForSettingsRestart(
  options: RestartWaitOptions = {},
): Promise<SettingsRestartResult> {
  const loadSettings = options.loadSettings ?? getSettings
  const wait = options.wait ?? delay
  const attempts = options.attempts ?? restartPollAttempts

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await wait(restartPollIntervalMs)
    try {
      const snapshot = await loadSettings()
      if (!snapshot.settings.restart.required) {
        return { status: 'complete', snapshot }
      }
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return { status: 'authentication-required' }
      }
      if (!isTemporaryRestartError(error)) {
        throw error
      }
    }
  }

  throw new SettingsRestartTimeoutError()
}

export async function waitForRestartTarget(
  target: string,
  options: Pick<RestartWaitOptions, 'wait' | 'attempts'> & {
    probe?: (url: string) => Promise<void>
  } = {},
): Promise<void> {
  const wait = options.wait ?? delay
  const attempts = options.attempts ?? restartPollAttempts
  const probe = options.probe ?? probeRestartTarget
  const checkURL = new URL('/api/connection/check', target).toString()

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await wait(restartPollIntervalMs)
    try {
      await probe(checkURL)
      return
    } catch {
      // A managed restart is expected to make the target briefly unreachable.
    }
  }

  throw new SettingsRestartTimeoutError()
}

export function settingsReconnectTarget(reconnectOrigin?: string): string {
  const candidate = reconnectOrigin?.trim() ?? ''
  if (candidate === '' || candidate === window.location.origin) return ''
  return new URL(
    `${window.location.pathname}${window.location.search}${window.location.hash}`,
    candidate,
  ).toString()
}

export function navigateToRestartTarget(target: string) {
  window.location.replace(target)
}

export function reloadForAuthentication() {
  window.location.reload()
}

function isTemporaryRestartError(error: unknown): boolean {
  if (error instanceof TypeError) return true
  if (!(error instanceof ApiError)) return false
  return error.status === 502 || error.status === 503 || error.status === 504
}

function delay(delayMs: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, delayMs))
}

async function probeRestartTarget(url: string): Promise<void> {
  await fetch(url, {
    method: 'GET',
    mode: 'no-cors',
    credentials: 'omit',
    cache: 'no-store',
  })
}
