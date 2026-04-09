const DEFAULT_APP_NAME = 'Sentinel'

function normalizeHostname(hostname?: string | null): string {
  return (hostname ?? '').trim()
}

function setHeadMetaContent(
  doc: Document,
  name: string,
  content: string,
): void {
  const meta = doc.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!meta) {
    return
  }
  meta.setAttribute('content', content)
}

export function formatHeaderBrand(hostname?: string | null): string {
  return normalizeHostname(hostname) || DEFAULT_APP_NAME
}

export function formatInstalledAppName(hostname?: string | null): string {
  const trimmedHostname = normalizeHostname(hostname)
  if (trimmedHostname === '') {
    return DEFAULT_APP_NAME
  }
  return `${trimmedHostname} - ${DEFAULT_APP_NAME}`
}

export function formatInstalledAppShortName(hostname?: string | null): string {
  return normalizeHostname(hostname) || DEFAULT_APP_NAME
}

export function applyDocumentAppBrand(
  hostname?: string | null,
  doc: Document = document,
): void {
  const appName = formatInstalledAppName(hostname)
  doc.title = appName
  setHeadMetaContent(
    doc,
    'apple-mobile-web-app-title',
    formatInstalledAppShortName(hostname),
  )
  setHeadMetaContent(doc, 'application-name', appName)
}
