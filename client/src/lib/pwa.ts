const PWA_UPDATE_READY_EVENT = 'sentinel.pwa.update-ready'
const LOCALHOST_HOSTNAMES = new Set(['localhost', '127.0.0.1', '::1'])

let waitingServiceWorker: ServiceWorker | null = null
let reloadOnControllerChangeAttached = false

export type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{
    outcome: 'accepted' | 'dismissed'
    platform: string
  }>
}

function canRegisterServiceWorker(): boolean {
  if (!('serviceWorker' in navigator)) {
    return false
  }
  if (window.isSecureContext) {
    return true
  }
  return LOCALHOST_HOSTNAMES.has(window.location.hostname)
}

function notifyUpdateReady(worker: ServiceWorker | null): void {
  waitingServiceWorker = worker
  window.dispatchEvent(new Event(PWA_UPDATE_READY_EVENT))
}

function bindControllerChangeReload(): void {
  if (reloadOnControllerChangeAttached) {
    return
  }
  reloadOnControllerChangeAttached = true
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    window.location.reload()
  })
}

function watchRegistrationForUpdates(
  registration: ServiceWorkerRegistration,
): void {
  if (registration.waiting) {
    notifyUpdateReady(registration.waiting)
  }

  registration.addEventListener('updatefound', () => {
    const installing = registration.installing
    if (!installing) {
      return
    }
    installing.addEventListener('statechange', () => {
      if (
        installing.state === 'installed' &&
        navigator.serviceWorker.controller
      ) {
        notifyUpdateReady(installing)
      }
    })
  })
}

let swRegistrationStarted = false

export function registerSentinelPwa(): void {
  if (swRegistrationStarted || !canRegisterServiceWorker()) {
    return
  }
  swRegistrationStarted = true

  const register = async () => {
    try {
      const registration = await navigator.serviceWorker.register('/sw.js', {
        scope: '/',
        updateViaCache: 'none',
      })
      bindControllerChangeReload()
      watchRegistrationForUpdates(registration)
      void registration.update()
    } catch {
      // PWA registration is best-effort and should never block app boot.
    }
  }

  if (document.readyState === 'complete') {
    void register()
    return
  }
  window.addEventListener('load', () => void register(), { once: true })
}

export function applySentinelPwaUpdate(): boolean {
  if (waitingServiceWorker === null) {
    return false
  }
  waitingServiceWorker.postMessage({ type: 'SKIP_WAITING' })
  return true
}

export function hasSentinelPwaUpdate(): boolean {
  return waitingServiceWorker !== null
}

export function getPwaUpdateReadyEventName(): string {
  return PWA_UPDATE_READY_EVENT
}

export type CheckForUpdateResult =
  | 'applied'
  | 'no-update'
  | 'unsupported'
  | 'failed'

const INSTALL_TIMEOUT_MS = 10_000

function activateWaitingWorker(worker: ServiceWorker): void {
  waitingServiceWorker = worker
  worker.postMessage({ type: 'SKIP_WAITING' })
}

async function waitForInstall(installing: ServiceWorker): Promise<boolean> {
  if (installing.state === 'installed') return true
  if (installing.state === 'activated' || installing.state === 'redundant') {
    return installing.state === 'activated'
  }
  return new Promise<boolean>((resolve) => {
    const onStateChange = () => {
      if (installing.state === 'installed') {
        installing.removeEventListener('statechange', onStateChange)
        window.clearTimeout(timer)
        resolve(true)
      } else if (installing.state === 'redundant') {
        installing.removeEventListener('statechange', onStateChange)
        window.clearTimeout(timer)
        resolve(false)
      }
    }
    installing.addEventListener('statechange', onStateChange)
    const timer = window.setTimeout(() => {
      installing.removeEventListener('statechange', onStateChange)
      resolve(false)
    }, INSTALL_TIMEOUT_MS)
  })
}

// Actively probe for a new service worker and activate it if one is
// waiting. Returns 'applied' when a new worker has been told to
// skipWaiting — the page will reload via the controllerchange listener.
// Returns 'no-update' when the server serves the same worker bytes.
export async function checkAndApplyPwaUpdate(): Promise<CheckForUpdateResult> {
  if (!canRegisterServiceWorker()) {
    return 'unsupported'
  }
  try {
    const registration = await navigator.serviceWorker.getRegistration()
    if (!registration) {
      return 'unsupported'
    }
    bindControllerChangeReload()

    if (registration.waiting) {
      activateWaitingWorker(registration.waiting)
      return 'applied'
    }

    await registration.update()

    if (registration.waiting) {
      activateWaitingWorker(registration.waiting)
      return 'applied'
    }

    const installing = registration.installing
    if (installing) {
      const installed = await waitForInstall(installing)
      if (installed && registration.waiting) {
        activateWaitingWorker(registration.waiting)
        return 'applied'
      }
    }

    return 'no-update'
  } catch {
    return 'failed'
  }
}
