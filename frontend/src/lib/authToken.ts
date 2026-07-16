import type { QueryClient } from '@tanstack/react-query'

export type AuthCookieUpdateResult = {
  ok: boolean
  status: number
  code: string
  message: string
}

type ErrorPayload = {
  error?: {
    code?: unknown
    message?: unknown
  }
}

export async function updateAuthCookie(
  queryClient: QueryClient,
  rawToken: string,
): Promise<AuthCookieUpdateResult> {
  const token = rawToken.trim()
  const headers: Record<string, string> = { Accept: 'application/json' }
  const request: RequestInit = {
    method: token === '' ? 'DELETE' : 'PUT',
    credentials: 'same-origin',
    headers,
  }
  if (token !== '') {
    headers['Content-Type'] = 'application/json'
    request.body = JSON.stringify({ token })
  }

  let response: Response | null = null
  let code = ''
  let message = ''
  try {
    response = await fetch('/api/auth/token', request)
    if (!response.ok) {
      const error = await readErrorPayload(response)
      code = error.code
      message = error.message
    }
  } catch {
    response = null
  }

  await queryClient.invalidateQueries({
    queryKey: ['meta'],
    exact: true,
  })
  await queryClient.refetchQueries({
    queryKey: ['meta'],
    exact: true,
    type: 'active',
  })

  return {
    ok: response?.ok === true,
    status: response?.status ?? 0,
    code,
    message,
  }
}

export function authCookieUpdateErrorMessage(
  result: AuthCookieUpdateResult,
  options?: { action?: 'validate' | 'clear' },
): string {
  const action = options?.action ?? 'validate'
  const fallback =
    action === 'clear' ? 'Unable to clear token right now.' : 'Unable to validate token right now.'

  if (action === 'validate' && result.status === 401) {
    return 'Invalid token.'
  }
  if (result.code === 'UNTRUSTED_PROXY') {
    return result.message || 'The HTTPS proxy is not listed in server.trusted_proxies.'
  }
  if (result.code === 'ORIGIN_DENIED') {
    return result.message || 'This browser origin is not listed in server.allowed_origins.'
  }
  if (result.status === 403) {
    return result.message || 'Sentinel rejected this browser origin.'
  }
  if (result.message !== '') {
    return result.message
  }
  if (result.status > 0) {
    return `${fallback} (HTTP ${result.status}).`
  }
  return fallback
}

async function readErrorPayload(response: Response): Promise<{ code: string; message: string }> {
  const contentType = response.headers.get('Content-Type') ?? ''
  if (!contentType.toLowerCase().includes('application/json')) {
    return { code: '', message: '' }
  }

  try {
    const payload = (await response.json()) as ErrorPayload
    return {
      code: typeof payload.error?.code === 'string' ? payload.error.code.trim() : '',
      message: typeof payload.error?.message === 'string' ? payload.error.message.trim() : '',
    }
  } catch {
    return { code: '', message: '' }
  }
}
