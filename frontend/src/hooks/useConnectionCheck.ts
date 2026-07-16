import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
  const [ready, setReady] = useState(false)
  const [checking, setChecking] = useState(false)
  const [issue, setIssue] = useState<ConnectionIssue | null>(null)
  const onUnauthorizedRef = useRef(onUnauthorized)
  onUnauthorizedRef.current = onUnauthorized

  const retry = useCallback(() => {
    setEpoch((value) => value + 1)
  }, [])

  useEffect(() => {
    if (!enabled) {
      setReady(false)
      setChecking(false)
      setIssue(null)
      return
    }

    const controller = new AbortController()
    setReady(false)
    setChecking(true)
    setIssue(null)

    void fetch('/api/connection/check', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
      .then(async (response) => {
        if (response.ok) {
          setReady(true)
          return
        }
        const nextIssue = await responseIssue(response)
        setIssue(nextIssue)
        if (response.status === 401) {
          onUnauthorizedRef.current()
        }
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') {
          return
        }
        setIssue({
          code: 'CONNECTION_FAILED',
          title: 'Sentinel is unreachable',
          message: 'The server did not answer the connection check.',
          configPath: '',
          configuration: '',
        })
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setChecking(false)
        }
      })

    return () => controller.abort()
  }, [enabled, epoch])

  useEffect(() => {
    const handleOnline = () => retry()
    window.addEventListener('online', handleOnline)
    return () => window.removeEventListener('online', handleOnline)
  }, [retry])

  return useMemo(() => ({ ready, checking, issue, retry }), [checking, issue, ready, retry])
}
