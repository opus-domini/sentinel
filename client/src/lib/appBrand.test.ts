// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'

import {
  applyDocumentAppBrand,
  formatHeaderBrand,
  formatInstalledAppName,
  formatInstalledAppShortName,
} from './appBrand'

describe('appBrand', () => {
  it('falls back to Sentinel when the hostname is missing', () => {
    expect(formatHeaderBrand()).toBe('Sentinel')
    expect(formatInstalledAppName('')).toBe('Sentinel')
    expect(formatInstalledAppShortName(null)).toBe('Sentinel')
  })

  it('keeps the hostname visible in the UI and installed app name', () => {
    expect(formatHeaderBrand('drako')).toBe('drako')
    expect(formatInstalledAppName('drako')).toBe('drako - Sentinel')
    expect(formatInstalledAppShortName('drako')).toBe('drako')
  })

  it('updates the document title and install-related meta tags', () => {
    document.head.innerHTML = `
      <meta name="application-name" content="Sentinel" />
      <meta name="apple-mobile-web-app-title" content="Sentinel" />
    `
    document.title = 'Sentinel'

    applyDocumentAppBrand('drako')

    expect(document.title).toBe('drako - Sentinel')
    expect(
      document.head
        .querySelector('meta[name="application-name"]')
        ?.getAttribute('content'),
    ).toBe('drako - Sentinel')
    expect(
      document.head
        .querySelector('meta[name="apple-mobile-web-app-title"]')
        ?.getAttribute('content'),
    ).toBe('drako')
  })
})
