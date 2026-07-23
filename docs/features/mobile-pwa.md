# Mobile and PWA

<p align="center">
  <img src="assets/images/mobile-tmux.png" alt="Mobile tmux view" width="320" />
</p>

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

- A single-row, 44 px terminal toolbar with fixed `Enter`, `Select`, and keyboard controls.
- Sticky `Ctrl`, `Alt`, and `Shift` modifiers. Tap to apply a modifier to the next eligible
  key, or hold it to keep the modifier locked until it is toggled off.
- An always-visible `More` action with Shift, arrows, Home, End, Page Up, Page Down,
  `/`, and `-`. The compact panel leaves ordinary numbers to the mobile keyboard, and the main
  bar does not depend on hidden horizontal scrolling.
- Connection-aware terminal keys. PTY actions are disabled while the terminal is reconnecting,
  while selection and keyboard controls remain available.
- Touch-to-wheel bridge for one-finger terminal scroll. Scrolling remains the default gesture.
- An explicit text-selection mode. Tap `Select`, drag over terminal text, then use `Copy` or
  `Cancel`. Selection can continue into scrollback by holding the drag near the top or bottom
  edge.
- A shared compact viewport policy that preserves the mobile shell in portrait and phone
  landscape orientations.
- Bottom gesture gutter protection to avoid accidental system gestures.
- Keyboard-aware viewport tracking (`visualViewport`) with CSS variables.
- Touch lock zones using `data-sentinel-touch-lock`.
- Keyboard-open behavior stabilization that hides secondary footer details and the bottom
  navigation to preserve terminal space.
- Touch-safe session and pane actions: close and split commands remain available in menus
  without adjacent 20 px destructive targets.
- WebSocket force-reconnect on `visibilitychange` — both the events channel and PTY stream reconnect when the tab becomes visible again, ensuring state is current after returning from background.

### Selecting and copying terminal text

1. Tap `Select` in the terminal toolbar.
2. Drag from the first character to the last. Dragging in either direction is supported.
3. Tap `Copy` when the range is highlighted, or `Cancel` to return without copying.

Selection temporarily takes precedence over terminal scroll and edge-swipe navigation. Leaving
selection mode restores the normal one-finger scroll behavior.

## Deployment Notes

PWA registration only runs in secure contexts (HTTPS) or localhost variants.
