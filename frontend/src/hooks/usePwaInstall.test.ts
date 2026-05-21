// @vitest-environment jsdom
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePwaInstall } from './usePwaInstall'
import { applySentinelPwaUpdate, checkAndApplyPwaUpdate } from '@/lib/pwa'

vi.mock('@/lib/pwa', () => ({
  applySentinelPwaUpdate: vi.fn(() => true),
  checkAndApplyPwaUpdate: vi.fn(async () => 'applied' as const),
  getPwaUpdateReadyEventName: () => 'sentinel.pwa.update-ready',
  hasSentinelPwaUpdate: () => false,
}))

function buildMatchMedia(matches = false): typeof window.matchMedia {
  return (() => ({
    matches,
    media: '(display-mode: standalone)',
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(() => true),
  })) as unknown as typeof window.matchMedia
}

describe('usePwaInstall', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('matchMedia', buildMatchMedia(false))
  })

  it('exposes install availability after beforeinstallprompt', () => {
    const { result } = renderHook(() => usePwaInstall())
    expect(result.current.installAvailable).toBe(false)

    const installEvent = new Event('beforeinstallprompt') as Event & {
      prompt: () => Promise<void>
      userChoice: Promise<{
        outcome: 'accepted' | 'dismissed'
        platform: string
      }>
    }
    installEvent.prompt = vi.fn(() => Promise.resolve())
    installEvent.userChoice = Promise.resolve({
      outcome: 'accepted',
      platform: 'web',
    })

    act(() => {
      window.dispatchEvent(installEvent)
    })

    expect(result.current.installAvailable).toBe(true)
  })

  it('prompts install and clears availability when accepted', async () => {
    const { result } = renderHook(() => usePwaInstall())
    const prompt = vi.fn(() => Promise.resolve())
    const installEvent = new Event('beforeinstallprompt') as Event & {
      prompt: () => Promise<void>
      userChoice: Promise<{
        outcome: 'accepted' | 'dismissed'
        platform: string
      }>
    }
    installEvent.prompt = prompt
    installEvent.userChoice = Promise.resolve({
      outcome: 'accepted',
      platform: 'web',
    })

    act(() => {
      window.dispatchEvent(installEvent)
    })

    await act(async () => {
      const installed = await result.current.installApp()
      expect(installed).toBe(true)
    })

    expect(prompt).toHaveBeenCalledTimes(1)
    expect(result.current.installAvailable).toBe(false)
  })

  it('marks app as installed after appinstalled event', () => {
    const { result } = renderHook(() => usePwaInstall())
    expect(result.current.installed).toBe(false)

    act(() => {
      window.dispatchEvent(new Event('appinstalled'))
    })

    expect(result.current.installed).toBe(true)
  })

  it('exposes update availability and applies updates', () => {
    const { result } = renderHook(() => usePwaInstall())
    expect(result.current.updateAvailable).toBe(false)

    act(() => {
      window.dispatchEvent(new Event('sentinel.pwa.update-ready'))
    })

    expect(result.current.updateAvailable).toBe(true)
    expect(result.current.applyUpdate()).toBe(true)
    expect(applySentinelPwaUpdate).toHaveBeenCalledTimes(1)
  })

  it('checkForUpdate keeps updating=true when an update is applied', async () => {
    vi.mocked(checkAndApplyPwaUpdate).mockResolvedValueOnce('applied')
    const { result } = renderHook(() => usePwaInstall())

    let outcome: string | undefined
    await act(async () => {
      outcome = await result.current.checkForUpdate()
    })

    expect(outcome).toBe('applied')
    // Page will reload via controllerchange — the button stays disabled
    // (updating = true) until the reload kicks in.
    expect(result.current.updating).toBe(true)
    expect(result.current.updateAvailable).toBe(false)
  })

  it('checkForUpdate clears updating when no update is available', async () => {
    vi.mocked(checkAndApplyPwaUpdate).mockResolvedValueOnce('no-update')
    const { result } = renderHook(() => usePwaInstall())

    let outcome: string | undefined
    await act(async () => {
      outcome = await result.current.checkForUpdate()
    })

    expect(outcome).toBe('no-update')
    await waitFor(() => {
      expect(result.current.updating).toBe(false)
    })
  })

  it('checkForUpdate clears updating on unsupported or failed results', async () => {
    vi.mocked(checkAndApplyPwaUpdate).mockResolvedValueOnce('unsupported')
    const { result } = renderHook(() => usePwaInstall())

    await act(async () => {
      await result.current.checkForUpdate()
    })

    expect(result.current.updating).toBe(false)
  })
})
