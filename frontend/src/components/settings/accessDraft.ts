import type { SettingsPatch, SettingsResponse } from '@/api/settings'
import type { ApiError } from '@/hooks/useTmuxApi'
import type { SecretIntent } from './SecretSettingControl'
import { readSettingsIssues } from './settingsIssues'

export type AccessDraft = {
  host: string
  port: string
  tokenIntent: SecretIntent
  tokenValue: string
  allowedOrigins: Array<string>
  trustedProxies: Array<string>
  cookieSecure: string
  allowInsecureCookie: boolean
}

export type AccessDraftKey =
  | 'host'
  | 'port'
  | 'token'
  | 'allowedOrigins'
  | 'trustedProxies'
  | 'cookieSecure'
  | 'allowInsecureCookie'

export type AccessDraftChange = {
  key: AccessDraftKey
  configKey: string
  before: string
  after: string
}

export type AccessDraftErrors = Partial<Record<AccessDraftKey | 'reconnectOrigin', string>>

export type BrowserEndpoint = {
  protocol: string
  hostname: string
}

export function accessDraftFromSettings(access: SettingsResponse['access']): AccessDraft {
  return {
    host: access.listener.host.effectiveValue,
    port: String(access.listener.port.effectiveValue),
    tokenIntent: 'keep',
    tokenValue: '',
    allowedOrigins: [...access.origins.allowed.effectiveValue],
    trustedProxies: [...access.proxies.trusted.effectiveValue],
    cookieSecure: access.cookies.secure.effectiveValue,
    allowInsecureCookie: access.cookies.allowInsecure.effectiveValue,
  }
}

export function diffAccessDraft(
  base: AccessDraft,
  draft: AccessDraft,
  access: SettingsResponse['access'],
): Array<AccessDraftChange> {
  const changes: Array<AccessDraftChange> = []
  addStringChange(changes, 'host', 'server.host', base.host, draft.host)
  addStringChange(changes, 'port', 'server.port', base.port, draft.port)
  if (draft.tokenIntent !== 'keep') {
    changes.push({
      key: 'token',
      configKey: 'server.token',
      before: access.authentication.token.configured ? 'Configured' : 'Not configured',
      after: draft.tokenIntent === 'replace' ? 'Replacement staged' : 'Clear',
    })
  }
  addListChange(
    changes,
    'allowedOrigins',
    'server.allowed_origins',
    base.allowedOrigins,
    draft.allowedOrigins,
  )
  addListChange(
    changes,
    'trustedProxies',
    'server.trusted_proxies',
    base.trustedProxies,
    draft.trustedProxies,
  )
  addStringChange(
    changes,
    'cookieSecure',
    'server.cookie_secure',
    base.cookieSecure,
    draft.cookieSecure,
  )
  if (base.allowInsecureCookie !== draft.allowInsecureCookie) {
    changes.push({
      key: 'allowInsecureCookie',
      configKey: 'server.allow_insecure_cookie',
      before: base.allowInsecureCookie ? 'Enabled' : 'Disabled',
      after: draft.allowInsecureCookie ? 'Enabled' : 'Disabled',
    })
  }
  return changes
}

export function candidateReconnectOrigin(endpoint: BrowserEndpoint, draft: AccessDraft): string {
  const port = parsePort(draft.port)
  if (port == null) return ''
  const candidateHost = draft.host.trim()
  const hostname =
    classifyListenerHost(candidateHost) === 'wildcard'
      ? endpoint.hostname.trim().replace(/^\[|\]$/g, '')
      : candidateHost.replace(/^\[|\]$/g, '')
  if (hostname === '') return ''
  const protocol = endpoint.protocol.replace(/:$/, '').toLowerCase()
  if (protocol !== 'http' && protocol !== 'https') return ''
  const urlHost = hostname.includes(':') ? `[${hostname}]` : hostname
  return `${protocol}://${urlHost}:${port}`
}

export function classifyListenerHost(host: string): 'loopback' | 'wildcard' | 'specific' {
  const normalized = host
    .trim()
    .replace(/^\[|\]$/g, '')
    .toLowerCase()
  if (normalized === '' || normalized === '0.0.0.0' || normalized === '::') return 'wildcard'
  if (
    normalized === 'localhost' ||
    normalized === '127.0.0.1' ||
    normalized === '::1' ||
    normalized.startsWith('127.')
  ) {
    return 'loopback'
  }
  return 'specific'
}

export function validateAccessDraft(
  draft: AccessDraft,
  access: SettingsResponse['access'],
  endpoint: BrowserEndpoint,
): AccessDraftErrors {
  const errors: AccessDraftErrors = {}
  if (draft.host.trim() === '') errors.host = 'Enter a listener host.'
  if (parsePort(draft.port) == null) {
    errors.port = `Enter a whole port from ${access.listener.port.validation.min} to ${access.listener.port.validation.max}.`
  }
  if (draft.tokenIntent === 'replace' && draft.tokenValue.trim() === '') {
    errors.token = 'Enter a non-empty replacement token.'
  }
  const originError = validateExactList(draft.allowedOrigins, validateOrigin)
  if (originError !== '') errors.allowedOrigins = originError
  const proxyError = validateExactList(draft.trustedProxies)
  if (proxyError !== '') errors.trustedProxies = proxyError

  const reconnectOrigin = candidateReconnectOrigin(endpoint, draft)
  if (reconnectOrigin === '') {
    errors.reconnectOrigin = 'Sentinel could not derive a valid reconnect origin.'
  }

  const remote = classifyListenerHost(draft.host) !== 'loopback'
  const tokenConfigured =
    draft.tokenIntent === 'replace' ||
    (draft.tokenIntent === 'keep' && access.authentication.token.configured)
  if (remote && !tokenConfigured) {
    errors.token =
      draft.tokenIntent === 'clear'
        ? 'A remote listener cannot clear its authentication token.'
        : 'A remote listener requires an authentication token.'
  }
  if (remote && draft.allowedOrigins.length === 0) {
    errors.allowedOrigins = 'A remote listener requires at least one allowed origin.'
  }
  if (tokenConfigured && draft.cookieSecure === 'always' && endpoint.protocol !== 'https:') {
    errors.cookieSecure =
      'Always secure cookies require the current browser connection to use HTTPS.'
  }
  if (remote && tokenConfigured && draft.cookieSecure === 'never' && !draft.allowInsecureCookie) {
    errors.allowInsecureCookie =
      'Remote authentication with never-secure cookies requires explicit insecure-cookie consent.'
  }
  return errors
}

export function accessPatchFromChanges(
  draft: AccessDraft,
  changes: Array<AccessDraftChange>,
  reconnectOrigin: string,
): SettingsPatch {
  const changed = new Set(changes.map((change) => change.key))
  const access: NonNullable<SettingsPatch['access']> = { reconnectOrigin }
  if (changed.has('host')) access.host = draft.host.trim()
  if (changed.has('port')) access.port = Number(draft.port)
  if (changed.has('token')) {
    access.token =
      draft.tokenIntent === 'replace'
        ? { action: 'replace', value: draft.tokenValue.trim() }
        : { action: 'clear' }
  }
  if (changed.has('allowedOrigins')) access.allowedOrigins = [...draft.allowedOrigins]
  if (changed.has('trustedProxies')) access.trustedProxies = [...draft.trustedProxies]
  if (changed.has('cookieSecure')) access.cookieSecure = draft.cookieSecure
  if (changed.has('allowInsecureCookie')) {
    access.allowInsecureCookie = draft.allowInsecureCookie
  }
  return { access }
}

export function accessErrorsFromAPI(error: ApiError): AccessDraftErrors {
  const issues = readSettingsIssues(error.details)
  const result: AccessDraftErrors = {}
  for (const issue of issues) {
    if (issue.includes('server.host') || issue.includes('listener preflight')) result.host = issue
    if (issue.includes('server.port') || issue.includes('candidate address')) result.port = issue
    if (issue.includes('server.token') || issue.includes('token is required')) result.token = issue
    if (issue.includes('server.allowed_origins') || issue.includes('allowed_origin')) {
      result.allowedOrigins = issue
    }
    if (issue.includes('server.trusted_proxies')) result.trustedProxies = issue
    if (issue.includes('server.cookie_secure')) result.cookieSecure = issue
    if (issue.includes('server.allow_insecure_cookie')) result.allowInsecureCookie = issue
    if (issue.includes('access.reconnectOrigin')) result.reconnectOrigin = issue
  }
  return result
}

function addStringChange(
  changes: Array<AccessDraftChange>,
  key: AccessDraftKey,
  configKey: string,
  before: string,
  after: string,
) {
  if (before === after) return
  changes.push({ key, configKey, before: before || 'Empty', after: after || 'Empty' })
}

function addListChange(
  changes: Array<AccessDraftChange>,
  key: AccessDraftKey,
  configKey: string,
  before: Array<string>,
  after: Array<string>,
) {
  if (before.length === after.length && before.every((value, index) => value === after[index])) {
    return
  }
  changes.push({
    key,
    configKey,
    before: listSummary(before),
    after: listSummary(after),
  })
}

function listSummary(values: Array<string>): string {
  return values.length === 0 ? 'Empty' : values.join(', ')
}

function parsePort(raw: string): number | null {
  if (!/^\d+$/.test(raw.trim())) return null
  const value = Number(raw)
  return Number.isInteger(value) && value >= 1 && value <= 65535 ? value : null
}

function validateExactList(values: Array<string>, validate?: (value: string) => string): string {
  const seen = new Set<string>()
  for (const value of values) {
    if (value === '' || value !== value.trim()) return 'Entries must be non-empty and trimmed.'
    if (seen.has(value)) return `Duplicate entry: ${value}`
    seen.add(value)
    const error = validate?.(value) ?? ''
    if (error !== '') return error
  }
  return ''
}

function validateOrigin(value: string): string {
  try {
    const parsed = new URL(value)
    if (
      (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
      parsed.username !== '' ||
      parsed.password !== '' ||
      (parsed.pathname !== '' && parsed.pathname !== '/') ||
      parsed.search !== '' ||
      parsed.hash !== ''
    ) {
      return `${value} must be an HTTP(S) origin without credentials, path, query, or fragment.`
    }
    return ''
  } catch {
    return `${value} must be an absolute HTTP(S) origin.`
  }
}
