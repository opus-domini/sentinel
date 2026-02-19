import { useCallback, useEffect, useMemo, useState } from 'react'
import type { BeforeInstallPromptEvent } from '@/lib/pwa'
import {
  applySentinelPwaUpdate,
  getPwaUpdateReadyEventName,
  hasSentinelPwaUpdate,
} from '@/lib/pwa'

function isStandaloneDisplayMode(): boolean {
  const media = window.matchMedia('(display-mode: standalone)')
  const navigatorWithStandalone = navigator as Navigator & {
    standalone?: boolean
  }
  return media.matches || navigatorWithStandalone.standalone === true
}

function canUsePwaFeatures(): boolean {
  if (window.isSecureContext) {
    return true
  }
  return (
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1' ||
    window.location.hostname === '::1'
  )
}

export function usePwaInstall() {
  const [installEvent, setInstallEvent] =
    useState<BeforeInstallPromptEvent | null>(null)
  const [installed, setInstalled] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [updateAvailable, setUpdateAvailable] = useState(() =>
    hasSentinelPwaUpdate(),
  )

  useEffect(() => {
    setInstalled(isStandaloneDisplayMode())

    const onBeforeInstallPrompt = (event: Event) => {
      const promptEvent = event as BeforeInstallPromptEvent
      promptEvent.preventDefault()
      setInstallEvent(promptEvent)
    }

    const onAppInstalled = () => {
      setInstalled(true)
      setInstallEvent(null)
    }

    const onUpdateReady = () => {
      setUpdateAvailable(true)
    }

    window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt)
    window.addEventListener('appinstalled', onAppInstalled)
    window.addEventListener(getPwaUpdateReadyEventName(), onUpdateReady)

    return () => {
      window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt)
      window.removeEventListener('appinstalled', onAppInstalled)
      window.removeEventListener(getPwaUpdateReadyEventName(), onUpdateReady)
    }
  }, [])

  const installAvailable = useMemo(
    () => installEvent !== null && !installed && canUsePwaFeatures(),
    [installEvent, installed],
  )

  const installApp = useCallback(async (): Promise<boolean> => {
    if (!installEvent) {
      return false
    }
    await installEvent.prompt()
    const choice = await installEvent.userChoice
    if (choice.outcome === 'accepted') {
      setInstallEvent(null)
      return true
    }
    return false
  }, [installEvent])

  const applyUpdate = useCallback((): boolean => {
    setUpdating(true)
    const applied = applySentinelPwaUpdate()
    if (!applied) {
      setUpdating(false)
    }
    return applied
  }, [])

  return {
    supportsPwa: canUsePwaFeatures(),
    installed,
    installAvailable,
    installApp,
    updateAvailable,
    applyUpdate,
    updating,
  }
}
