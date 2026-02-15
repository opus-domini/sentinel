# Mobile and PWA

Sentinel includes mobile-focused terminal interaction and Progressive Web App support.

## PWA Support

- Manifest: `/manifest.webmanifest`
- Service Worker: `/sw.js`
- App install prompt support (`beforeinstallprompt`)
- App update detection and apply flow

In Settings, users can install app and apply pending PWA updates.

## Offline Strategy

Service worker caches app shell and static assets.

- Network-first for page navigations
- Stale-while-revalidate for static assets
- `/api/*` and `/ws/*` are never cached

This keeps UI shell resilient without caching live terminal data paths.

## Mobile Terminal UX

Implemented improvements include:

- Touch-to-wheel bridge for terminal scroll.
- Bottom gesture gutter protection to avoid accidental system gestures.
- Keyboard-aware viewport tracking (`visualViewport`) with CSS variables.
- Touch lock zones using `data-sentinel-touch-lock`.
- Keyboard-open behavior stabilization to avoid layout drift and unwanted scroll.

## Deployment Notes

PWA registration only runs in secure contexts (HTTPS) or localhost variants.
