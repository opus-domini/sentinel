import { useCallback, useEffect, useEffectEvent, useMemo, useState } from 'react'
import type { ConnectionHealth, ConnectionIssue } from '@/contexts/ConnectionHealthContext'

type ErrorPayload = {
  error?: {
    code?: unknown
    message?: unknown
    details?: {
      configPath?: unknown
      configuration?: unknown
    }
  }
}

function issueTitle(code: string): string {
  switch (code) {
    case 'UNTRUSTED_PROXY':
      return 'HTTPS proxy is not trusted'
    case 'ORIGIN_DENIED':
      return 'Browser origin is not allowed'
    case 'UNAUTHORIZED':
      return 'Authentication expired'
    default:
      return 'Connection check failed'
  }
}

async function responseIssue(response: Response): Promise<ConnectionIssue> {
  let payload: ErrorPayload = {}
  try {
    payload = (await response.json()) as ErrorPayload
  } catch {
    // The HTTP status below still provides a useful deterministic failure.
  }

  const code =
    typeof payload.error?.code === 'string' && payload.error.code.trim() !== ''
      ? payload.error.code.trim()
      : `HTTP_${response.status}`
  const message =
    typeof payload.error?.message === 'string' && payload.error.message.trim() !== ''
      ? payload.error.message.trim()
      : `Sentinel rejected the connection check with HTTP ${response.status}.`
  const configPath =
    typeof payload.error?.details?.configPath === 'string'
      ? payload.error.details.configPath.trim()
      : ''
  const configuration =
    typeof payload.error?.details?.configuration === 'string'
      ? payload.error.details.configuration.trim()
      : ''

  return {
    code,
    title: issueTitle(code),
    message,
    configPath,
    configuration,
  }
}

export function useConnectionCheck(options: {
  enabled: boolean
  onUnauthorized: () => void
}): ConnectionHealth {
  const { enabled, onUnauthorized } = options
  const [epoch, setEpoch] = useState(0)
  const requestKey = enabled ? epoch : -1
  const [state, setState] = useState(() => ({
    requestKey,
    ready: false,
    checking: enabled,
    issue: null as ConnectionIssue | null,
  }))
  const handleUnauthorized = useEffectEvent(onUnauthorized)

  if (state.requestKey !== requestKey) {
    // A re-check never regresses an approved session: dropping `ready` unmounts
    // the route tree, which destroys every open terminal and its scrollback.
    // Losing authorization (`enabled` going false) is the only way back to the
    // gate. Later failures surface through `issue` instead.
    setState({ requestKey, ready: enabled && state.ready, checking: enabled, issue: null })
  }

  const retry = useCallback(() => {
    setEpoch((value) => value + 1)
  }, [])

  useEffect(() => {
    if (!enabled) return

    const controller = new AbortController()

    void fetch('/api/connection/check', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
      .then(async (response) => {
        if (response.ok) {
          setState((current) =>
            current.requestKey === requestKey ? { ...current, ready: true } : current,
          )
          return
        }
        const nextIssue = await responseIssue(response)
        setState((current) =>
          current.requestKey === requestKey ? { ...current, issue: nextIssue } : current,
        )
        if (response.status === 401) {
          handleUnauthorized()
        }
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') {
          return
        }
        setState((current) =>
          current.requestKey === requestKey
            ? {
                ...current,
                issue: {
                  code: 'CONNECTION_FAILED',
                  title: 'Sentinel is unreachable',
                  message: 'The server did not answer the connection check.',
                  configPath: '',
                  configuration: '',
                },
              }
            : current,
        )
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setState((current) =>
            current.requestKey === requestKey ? { ...current, checking: false } : current,
          )
        }
      })

    return () => controller.abort()
  }, [enabled, requestKey])

  useEffect(() => {
    const handleOnline = () => retry()
    window.addEventListener('online', handleOnline)
    return () => window.removeEventListener('online', handleOnline)
  }, [retry])

  return useMemo(
    () => ({ ready: state.ready, checking: state.checking, issue: state.issue, retry }),
    [retry, state.checking, state.issue, state.ready],
  )
}
