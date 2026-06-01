import { useEffect, useRef, useState } from 'react'

import type { ConnectionState } from '@/types'
import { buildWSProtocols } from '@/lib/wsAuth'
import { createReconnect } from '@/lib/wsReconnect'

type LogStreamTarget =
  | { kind: 'service'; name: string }
  | { kind: 'unit'; unit: string; scope: string; manager: string }

type UseLogStreamOptions = {
  authenticated: boolean
  tokenRequired: boolean
  target: LogStreamTarget | null
  enabled: boolean
  onLine: (line: string) => void
}

function logStreamTargetKey(target: LogStreamTarget | null): string {
  if (target == null) return ''
  if (target.kind === 'service') return `service:${target.name}`
  return `unit:${target.manager}:${target.scope}:${target.unit}`
}

function logStreamTargetQuery(target: LogStreamTarget | null): string {
  if (target == null) return ''
  const params = new URLSearchParams()
  if (target.kind === 'service') {
    params.set('service', target.name)
  } else {
    params.set('unit', target.unit)
    params.set('scope', target.scope)
    params.set('manager', target.manager)
  }
  return params.toString()
}

function isPermanentClose(event: CloseEvent): boolean {
  const reason = event.reason.toLowerCase()
  if (event.code === 1000 && reason === 'done') return true
  return (
    reason.includes('stream start failed') ||
    reason.includes('auth') ||
    reason.includes('unauthorized') ||
    reason.includes('forbidden') ||
    reason.includes('policy')
  )
}

export function useLogStream({
  authenticated,
  tokenRequired,
  target,
  enabled,
  onLine,
}: UseLogStreamOptions): ConnectionState {
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected')
  const targetKey = logStreamTargetKey(target)
  const targetQuery = logStreamTargetQuery(target)

  const onLineRef = useRef(onLine)
  useEffect(() => {
    onLineRef.current = onLine
  }, [onLine])

  useEffect(() => {
    if (!enabled || targetKey === '') {
      setConnectionState('disconnected')
      return
    }
    if (tokenRequired && !authenticated) {
      setConnectionState('disconnected')
      return
    }

    let disposed = false
    let socket: WebSocket | null = null
    let retryTimer: number | null = null
    const reconnect = createReconnect()

    const clearRetry = () => {
      if (retryTimer != null) {
        window.clearTimeout(retryTimer)
        retryTimer = null
      }
    }

    const connect = () => {
      if (disposed) return
      clearRetry()
      setConnectionState('connecting')

      const wsURL = new URL(`/ws/logs?${targetQuery}`, window.location.origin)
      wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:'

      socket = new WebSocket(wsURL.toString(), buildWSProtocols())

      socket.onopen = () => {
        if (disposed) return
        reconnect.reset()
        setConnectionState('connected')
      }

      socket.onmessage = (event) => {
        if (disposed) return
        let msg: unknown
        try {
          msg = JSON.parse(String(event.data))
        } catch {
          return
        }
        if (typeof msg !== 'object' || msg === null) return
        const typed = msg as { type?: string; line?: string; message?: string }
        if (typed.type === 'log' && typeof typed.line === 'string') {
          onLineRef.current(typed.line)
        } else if (typed.type === 'error' && typeof typed.message === 'string') {
          disposed = true
          setConnectionState('error')
          clearRetry()
          try {
            socket?.close()
          } catch {
            // ignore close race
          }
        }
      }

      socket.onerror = () => {
        if (!disposed) {
          setConnectionState('error')
        }
      }

      socket.onclose = (event) => {
        if (disposed) return
        if (isPermanentClose(event)) {
          const done = event.code === 1000 && event.reason.toLowerCase() === 'done'
          setConnectionState(done ? 'disconnected' : 'error')
          clearRetry()
          return
        }
        setConnectionState('disconnected')
        clearRetry()
        retryTimer = window.setTimeout(connect, reconnect.next())
      }
    }

    connect()
    return () => {
      disposed = true
      clearRetry()
      if (socket != null) {
        try {
          socket.close()
        } catch {
          // ignore close race
        }
      }
    }
  }, [authenticated, tokenRequired, targetKey, targetQuery, enabled])

  return connectionState
}
