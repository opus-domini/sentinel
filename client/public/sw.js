const CACHE_VERSION = 'sentinel-v3'
const CORE_CACHE = `${CACHE_VERSION}-core`
const RUNTIME_CACHE = `${CACHE_VERSION}-runtime`
const APP_SHELL = '/index.html'

const CORE_URLS = [
  APP_SHELL,
  '/tmux',
  '/ops',
  '/assets/app.js',
  '/assets/app.css',
  '/assets/favicon.svg',
  '/manifest.webmanifest',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
]

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CORE_CACHE)
      .then((cache) => cache.addAll(CORE_URLS))
      .then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys()
      await Promise.all(
        keys
          .filter((key) => key !== CORE_CACHE && key !== RUNTIME_CACHE)
          .map((key) => caches.delete(key)),
      )
      await pruneInvalidRuntimeCache()
      await self.clients.claim()
    })(),
  )
})

self.addEventListener('message', (event) => {
  if (event.data?.type === 'SKIP_WAITING') {
    self.skipWaiting()
  }
})

function shouldBypass(requestUrl) {
  return (
    requestUrl.origin !== self.location.origin ||
    requestUrl.pathname.startsWith('/api') ||
    requestUrl.pathname.startsWith('/ws')
  )
}

function isRuntimeAssetPath(pathname) {
  return (
    pathname.startsWith('/assets/') ||
    pathname.startsWith('/icons/') ||
    pathname === '/manifest.webmanifest'
  )
}

function isHTMLResponse(response) {
  const contentType = response.headers.get('content-type') || ''
  return contentType.toLowerCase().includes('text/html')
}

function shouldCacheResponse(request, response) {
  if (!response || !response.ok) {
    return false
  }
  if (request.mode === 'navigate') {
    return true
  }
  const requestUrl = new URL(request.url)
  if (isRuntimeAssetPath(requestUrl.pathname) && isHTMLResponse(response)) {
    return false
  }
  return true
}

async function readValidCachedResponse(cache, request) {
  const cached = await cache.match(request)
  if (!cached) {
    return null
  }
  const requestUrl = new URL(request.url)
  if (
    isRuntimeAssetPath(requestUrl.pathname) &&
    (!cached.ok || isHTMLResponse(cached))
  ) {
    await cache.delete(request)
    return null
  }
  return cached
}

async function pruneInvalidRuntimeCache() {
  const cache = await caches.open(RUNTIME_CACHE)
  const keys = await cache.keys()
  await Promise.all(
    keys.map(async (request) => {
      const requestUrl = new URL(request.url)
      if (!isRuntimeAssetPath(requestUrl.pathname)) {
        return
      }
      const response = await cache.match(request)
      if (!response || !response.ok || isHTMLResponse(response)) {
        await cache.delete(request)
      }
    }),
  )
}

async function networkFirst(request) {
  try {
    const response = await fetch(request)
    if (shouldCacheResponse(request, response)) {
      const cache = await caches.open(RUNTIME_CACHE)
      cache.put(request, response.clone())
    }
    return response
  } catch {
    const cache = await caches.open(RUNTIME_CACHE)
    const cached = await readValidCachedResponse(cache, request)
    if (cached) {
      return cached
    }
    return caches.match(APP_SHELL)
  }
}

async function staleWhileRevalidate(request) {
  const cache = await caches.open(RUNTIME_CACHE)
  const cached = await readValidCachedResponse(cache, request)
  const networkPromise = fetch(request)
    .then((response) => {
      if (shouldCacheResponse(request, response)) {
        cache.put(request, response.clone())
      }
      return response
    })
    .catch(() => null)

  if (cached) {
    void networkPromise
    return cached
  }
  const network = await networkPromise
  if (network) {
    return network
  }
  return new Response('offline', {
    status: 503,
    statusText: 'Service Unavailable',
  })
}

self.addEventListener('fetch', (event) => {
  if (event.request.method !== 'GET') {
    return
  }

  const requestUrl = new URL(event.request.url)
  if (shouldBypass(requestUrl)) {
    return
  }

  if (event.request.mode === 'navigate') {
    event.respondWith(networkFirst(event.request))
    return
  }

  if (isRuntimeAssetPath(requestUrl.pathname)) {
    event.respondWith(staleWhileRevalidate(event.request))
  }
})
